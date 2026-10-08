# --- Core modules ---
module "network" {
  source                     = "./modules/network"
  project_name               = var.project_name
  vpc_cidr                   = var.vpc_cidr
  azs                        = var.azs
  public_subnet_cidrs        = var.public_subnet_cidrs
  private_subnet_cidrs       = var.private_subnet_cidrs
  one_nat_gateway            = var.one_nat_gateway
  create_interface_endpoints = true
  region                     = var.aws_region
}

module "dynamodb" {
  source       = "./modules/dynamodb"
  project_name = var.project_name
}

module "kinesis" {
  source           = "./modules/kinesis"
  environment      = var.project_name
  retention_period = 168  # 7 days
  shard_count      = 2
  tags = {
    ISBStudent = "true"
  }
}

module "ecs" {
  source             = "./modules/ecs"
  project_name       = var.project_name
  vpc_id             = module.network.vpc_id
  private_subnet_ids = module.network.private_subnet_ids
  public_subnet_ids  = module.network.public_subnet_ids
  alb_ingress_cidr            = var.alb_ingress_cidr
  create_alb                  = true
  aws_region                  = var.aws_region
  ddb_table_idempotency       = module.dynamodb.idempotency_table_name
  ddb_table_payments          = module.dynamodb.payments_table_name
  ddb_table_balances          = module.dynamodb.balances_table_name
  ecs_task_execution_role_arn = aws_iam_role.ecs_task_execution.arn
  ecs_task_role_arn           = aws_iam_role.ecs_task.arn
  kafka_bootstrap_servers     = ""  # Not using Kafka anymore
  kafka_topic                 = var.kafka_payments_topic
  processor_desired_count     = var.api_desired_count
  kinesis_stream_name         = module.kinesis.stream_name
  kinesis_checkpoint_table    = module.kinesis.checkpoint_table_name
}

# --- ECR ---
resource "aws_ecr_repository" "api" {
  name                 = "${var.project_name}-api"
  image_tag_mutability = "MUTABLE"
  image_scanning_configuration { scan_on_push = true }
  force_delete = true
}

# --- Build & push image with Docker provider ---
resource "docker_image" "api" {
  name = "${aws_ecr_repository.api.repository_url}:${var.api_image_tag}"
  build {
    context    = "${path.root}/../api"
    dockerfile = "${path.root}/../api/Dockerfile"
    platform   = "linux/amd64" # keep on Apple Silicon
  }
  depends_on = [aws_ecr_repository.api]
}

resource "docker_registry_image" "api" {
  name = docker_image.api.name
  depends_on = [aws_ecr_repository.api, docker_image.api]
}

# --- IAM Roles ---
# ECS Task Execution Role (for pulling images, writing logs)
resource "aws_iam_role" "ecs_task_execution" {
  name = "${var.project_name}-ecs-task-execution-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "ecs-tasks.amazonaws.com"
        }
      }
    ]
  })

  tags = {
    ISBStudent = "true"
  }
}

resource "aws_iam_role_policy_attachment" "ecs_task_execution_policy" {
  role       = aws_iam_role.ecs_task_execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

# ECS Task Role (for application to access AWS services)
# Using managed_policy_arns to attach policies atomically during role creation
resource "aws_iam_role" "ecs_task" {
  name = "${var.project_name}-ecs-task-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "ecs-tasks.amazonaws.com"
        }
      }
    ]
  })

  # Attach managed policies during role creation
  managed_policy_arns = [
    "arn:aws:iam::aws:policy/AmazonDynamoDBFullAccess",
    "arn:aws:iam::aws:policy/AmazonKinesisFullAccess"
  ]

  tags = {
    ISBStudent = "true"
  }
}

# --- Logs ---
resource "aws_cloudwatch_log_group" "api" {
  name              = "/ecs/${var.project_name}-api"
  retention_in_days = 7
}

# --- Task definition ---
locals {
  api_image_url = docker_image.api.name
}

resource "aws_ecs_task_definition" "api" {
  family                   = "${var.project_name}-api"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = var.api_cpu
  memory                   = var.api_memory
  execution_role_arn       = aws_iam_role.ecs_task_execution.arn
  task_role_arn            = aws_iam_role.ecs_task.arn

  # Force new revision when image changes
  lifecycle {
    replace_triggered_by = [docker_registry_image.api.sha256_digest]
  }

  container_definitions = jsonencode([{
    name  = "api"
    image = local.api_image_url
    essential   = true
    portMappings = [{ containerPort = 8080, hostPort = 8080, protocol = "tcp" }]
    environment = [
      { name = "PORT",                  value = "8080" },
      { name = "STORE_BACKEND",         value = "dynamo" },
      { name = "AWS_REGION",            value = var.aws_region },
      { name = "DDB_TABLE_IDEMPOTENCY", value = module.dynamodb.table_names.idempotency },
      { name = "DDB_TABLE_PAYMENTS",    value = module.dynamodb.table_names.payments },
      { name = "DDB_TABLE_BALANCES",    value = module.dynamodb.table_names.balances },

      # --- Broker wiring ---
      # Use Kinesis for immutable event log
      { name = "BROKER_BACKEND",           value = "kinesis" },
      { name = "KINESIS_STREAM_NAME",      value = module.kinesis.stream_name },
      { name = "KINESIS_CHECKPOINT_TABLE", value = module.kinesis.checkpoint_table_name }
    ]

    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-region        = var.aws_region
        awslogs-group         = aws_cloudwatch_log_group.api.name
        awslogs-stream-prefix = "api"
      }
    }
    # enable only if /admin/health exists
    # healthCheck = {
    #   command     = ["CMD-SHELL", "curl -fsS http://localhost:8080/admin/health || exit 1"]
    #   interval    = 10
    #   timeout     = 5
    #   retries     = 3
    #   startPeriod = 5
    # }
  }])

  depends_on = [docker_registry_image.api]
}

# --- Service (uses module.ecs ALB/TG) ---
resource "aws_ecs_service" "api" {
  name            = "${var.project_name}-api"
  cluster         = module.ecs.cluster_name
  task_definition = aws_ecs_task_definition.api.arn
  desired_count   = var.api_desired_count
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = module.network.private_subnet_ids
    security_groups  = [module.ecs.ecs_service_sg_id]
    assign_public_ip = false
  }

  load_balancer {
    target_group_arn = module.ecs.api_target_group_arn
    container_name   = "api"
    container_port   = 8080
  }

  lifecycle { ignore_changes = [task_definition] }
  depends_on = [module.ecs]
}

# MSK module commented out - using Kinesis instead
# module "msk" {
#   source             = "./modules/msk"
#   project_name       = var.project_name
#   vpc_id             = module.network.vpc_id
#   private_subnet_ids = module.network.private_subnet_ids
#   security_group_ids = [module.ecs.ecs_service_sg_id]
#   enable_msk         = var.enable_msk
#   msk_cluster_name   = "${var.project_name}-msk"
# }
