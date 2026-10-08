# **Event Payments System**

## **Overview**
This project implements a **distributed event-driven payment processing system** built in Go, deployed on AWS using Terraform and ECS Fargate.  
The system is designed to ensure **auditability, idempotency, and horizontal scalability** by treating every change as an immutable event and maintaining read models for fast queries.

---

## **Architecture at a Glance**

### **Core Components**
| Component | Purpose |
|------------|----------|
| **API Service (Go)** | Accepts payment-related requests (create, authorize, capture, fail). Enforces idempotency and durability by persisting immutable events. |
| **Processor (future)** | Consumes partitioned events from Kafka (MSK), applies business logic (authorize, capture, fail), and updates DynamoDB read models. |
| **Replay Tool (future)** | Rebuilds the read models from event logs or snapshots after schema or system changes. |
| **Read Models (DynamoDB)** | Serve low-latency reads for balances and payments while guaranteeing idempotent updates. |
| **Event Log (future)** | Amazon MSK Serverless Kafka will serve as the durable, append-only event backbone. |
| **Infrastructure (Terraform)** | Orchestrates AWS resources — VPC, subnets, ECS cluster, ALB, DynamoDB, IAM roles, and ECR repositories. |

---

## **Current Implementation (MVP)**
The current milestone focuses on deploying the **API Service** and persisting data to DynamoDB.

### **API Behavior**
- **POST /payments** → Creates a new payment request (idempotent).  
- **GET /payments/{paymentId}** → Fetches the latest view of a payment.  
- **POST /payments/{paymentId}/authorize** → Marks the payment as authorized.  
- **POST /payments/{paymentId}/capture** → Marks the payment as captured.  
- **POST /payments/{paymentId}/fail** → Marks the payment as failed.  
- **GET /accounts/{accountId}/balance** → Retrieves the current available and pending balances.

### **Idempotency**
Each POST/PUT request requires an `Idempotency-Key` header.  
The API ensures:
- Duplicate requests with the same key are ignored.
- The original `paymentId` and response are returned.
- A DynamoDB table (`idempotency`) maintains deduplication records with TTL.

### **Persistence**
- DynamoDB tables:
  - `Payments` — stores current payment state per `paymentId`.
  - `Balances` — tracks available/pending amounts per account.
  - `Idempotency` — enforces deduplication windows.
- Conditional writes (`attribute_not_exists` / `lastAppliedEvent`) ensure safe, idempotent updates.

---

## **Infrastructure (Terraform)**

### **Modules Used**
| Module | Role |
|--------|------|
| `network` | Creates VPC, subnets (public/private), route tables, NAT gateway, and VPC endpoints for DynamoDB, S3, CloudWatch, etc. |
| `ecs` | Sets up ECS Cluster (Fargate), ALB (Application Load Balancer), and security groups. |
| `dynamodb` | Creates tables for Idempotency, Payments, and Balances. |
| `msk` (optional) | Placeholder for future Kafka event log. |

### **Key AWS Resources**
- **VPC:** Private subnets host ECS tasks; public subnets host ALB.  
- **ALB:** Routes HTTP requests to ECS tasks (port 8080).  
- **ECS Fargate:** Runs the API container built from the Go service.  
- **CloudWatch Logs:** Collects structured JSON logs.  
- **ECR:** Stores Docker images (`api`, `processor`, `replay`).  
- **IAM (LabRole):** Grants ECS tasks access to DynamoDB and CloudWatch.  
- **SSM Parameters:** Define feature flags for enabling/disabling processors.  

### **Environment Variables Injected to API**
| Variable | Purpose |
|-----------|----------|
| `STORE_BACKEND=dynamo` | Enables persistent mode. |
| `AWS_REGION` | Configures AWS SDK region. |
| `DDB_TABLE_IDEMPOTENCY` | DynamoDB table for idempotency. |
| `DDB_TABLE_PAYMENTS` | DynamoDB table for payments. |
| `DDB_TABLE_BALANCES` | DynamoDB table for balances. |
| `PORT=8080` | Application port for ALB health checks. |

---

## **Deployment Flow**

### **1️. Build and Push API Image**
```bash
cd api
docker build -t event-payments-api:latest .
aws ecr get-login-password --region us-west-2 | docker login --username AWS <ECR_URL>
docker tag event-payments-api:latest <ECR_URL>:latest
docker push <ECR_URL>:latest
```

### **2. Deploy Infrastructure**
```bash
cd terraform
terraform init
terraform apply -auto-approve
```
### **3. Access API**
After apply completes, fetch the ALB DNS:
```bash
terraform output alb_dns_name
```
---

## **Testing**
Use Postman or curl to validate end-to-end:
### **Create Payment**
```bash
curl -X POST http://<ALB>/payments \
 -H "Content-Type: application/json" \
 -H "Idempotency-Key: test-123" \
 -d '{"accountId": "acc-001", "amount": 1000, "currency": "USD"}'
```

### **Get Payment**
```bash
curl http://<ALB>/payments/<paymentId>
```

### **Get Balance**
```bash
curl http://<ALB>/accounts/acc-001/balance
```
---
## **Observability**
- **Logs**: /ecs/event-payments-api CloudWatch Log Group
- **Health check**: /admin/health endpoint (used by ALB + ECS task)
- **Metrics**: CPU/Memory via ECS; DynamoDB read/write metrics; ALB latency.

---

## **Future Roadmap**

| **Phase** | **Focus Area** | **Key Milestones & Goals** |
|------------|----------------|-----------------------------|
| **Phase 1 – Current** | Deploy API with persistent DynamoDB backend | ECS Fargate deployment, ALB routing, DynamoDB persistence for Idempotency, Payments, and Balances. |
| **Phase 2 – Event Backbone** | Introduce Kafka (Amazon MSK Serverless) | Replace in-memory broker with MSK. All API writes produce immutable events to the `payments.events` topic (partitioned by `accountId`). |
| **Phase 3 – Processor Service** | Event-driven business logic | Add a separate ECS service (`payments-processor`) that consumes from MSK, applies state transitions (authorize, capture, fail), and updates DynamoDB read models idempotently. |
| **Phase 4 – Replay Tool** | State rebuild & recovery | Build an on-demand ECS task (`payments-replay`) that can reconstruct read models from earliest offsets or snapshots for auditing and recovery. |
| **Phase 5 – Observability & Scaling** | End-to-end monitoring and automation | Integrate OpenTelemetry tracing, MSK consumer lag metrics, CloudWatch dashboards, and ECS autoscaling based on queue depth and throughput. |
| **Phase 6 – Analytics & RDS Integration** | Add analytical insights layer | Mirror DynamoDB read models into Amazon RDS (Postgres) for ad-hoc queries, historical analysis, and compliance reports. |
| **Phase 7 – Multi-Tenancy & Security Hardening** | Tenant isolation and least-privilege IAM | Introduce `tenantId` in event schema, scoped IAM roles per service, and encryption-at-rest policies across all data stores. |
| **Phase 8 – CI/CD & Deployment Automation** | Streamline builds and rollouts | Add GitHub Actions or CodePipeline for automated builds, Docker pushes, and Terraform deployments with blue/green promotion. |
| **Phase 9 – Cost & Efficiency Optimization** | Optimize compute and data usage | Enable DynamoDB autoscaling, MSK topic compaction, and ECS task right-sizing for sustained throughput with minimal spend. |



---

## **Key Design Principles**
- **Idempotent from day one** — duplicate requests never cause double processing.  
- **Append-only semantics** — every business change is emitted as an immutable event, ensuring full auditability and replayability.  
- **Exactly-once effects via idempotent writes** — DynamoDB conditional expressions (`attribute_not_exists`, `lastAppliedEvent`) guarantee correctness across retries and failures.  
- **Separation of concerns** — API writes events, Processor applies logic, Replay rebuilds state; each can scale independently.  
- **Scalable & recoverable** — sharding by `accountId` enables independent parallel processing and replay for massive scale.  
- **Stateless compute, stateful persistence** — ECS tasks are ephemeral; all durable data lives in DynamoDB and event logs.  
- **Future-proof infrastructure** — Terraform modules isolate compute, storage, and messaging for safe incremental rollout.

---

## **Team Notes**
- Each Terraform module follows consistent naming: `${var.project_name}-<component>` for easy teardown and multi-environment support (`dev`, `perf`, `prod`).  
- API can run locally with `STORE_BACKEND=memory` for testing and development.  
- All ECS tasks, including future Processor and Replay services, will share the same private subnets and security groups for internal communication.  
- **LabRole** is used as a catch-all IAM role in the current setup for simplicity — future state should replace it with scoped IAM policies for least privilege.  
- **Runbooks** should be created for:  
  - Manual replay and state rebuilds  
  - Scaling partitions or consumer groups  
  - Draining DLQs and inspecting poison events  

---

## **Next Steps**
1. **Finalize event schema** for PaymentRequested / PaymentAuthorized / PaymentCaptured / PaymentFailed.  
2. **Integrate MSK Serverless** (Kafka) and move from in-memory broker → event log.  
3. **Implement Processor service** to consume from MSK and update DynamoDB.  
4. **Add Replay Tool** as an on-demand ECS task (Fargate one-off) triggered by `/admin/replay`.  
5. **Enhance Observability** — CloudWatch dashboards, DLQ metrics, OpenTelemetry traces.  
6. **Add CI/CD** using GitHub Actions or AWS CodePipeline for build, test, and deploy.  
7. **Enable Blue/Green Deployments** via ALB weighted target groups for zero-downtime upgrades.  
8. **Introduce Secrets Manager** for Kafka creds, database connections, and feature flags.  
9. **Add RDS Postgres (optional)** for analytical queries and reporting.  
10. **Enable cost controls** — DynamoDB autoscaling, ECS service autoscaling, and MSK retention tuning.

---

