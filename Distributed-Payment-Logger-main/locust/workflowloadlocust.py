from locust import HttpUser, task, between
import random

class PaymentWorkflowUser(HttpUser):
    wait_time = between(1, 2)
    
    def on_start(self):
        """Generate unique account ID for this user"""
        self.account_id = f"account-{random.randint(1000000, 9999999)}"
    
    @task
    def complete_payment_workflow(self):
        """Execute complete payment lifecycle: Create -> Authorize -> Capture -> Verify Balance"""
        
        # Generate unique payment request
        idempotency_key = f"idem-{random.randint(1000000000, 9999999999)}"
        amount = random.randint(100, 10000)
        
        # Step 1: Create payment
        create_payload = {
            "accountId": self.account_id,
            "amount": amount,
            "currency": "USD",
            "paymentMethodRef": f"pm-{random.randint(100000, 999999)}"
        }
        create_headers = {"Idempotency-Key": idempotency_key}
        
        create_response = self.client.post(
            "/payments",
            json=create_payload,
            headers=create_headers,
            name="POST /payments (Create)"
        )
        
        if create_response.status_code != 202:  # API returns 202 Accepted, not 201
            return
        
        payment_data = create_response.json()
        payment_id = payment_data.get("paymentId")  # API returns "paymentId" not "id"
        
        if not payment_id:
            return
        
        # Step 2: Authorize payment (simulate processing delay)
        self.wait()
        
        authorize_response = self.client.post(
            f"/payments/{payment_id}/authorize",
            name="POST /payments/:id/authorize"
        )
        
        if authorize_response.status_code != 202:  # API returns 202 Accepted
            return
        
        # Step 3: Capture payment (simulate processing delay)
        self.wait()
        
        capture_response = self.client.post(
            f"/payments/{payment_id}/capture",
            name="POST /payments/:id/capture"
        )
        
        if capture_response.status_code != 202:  # API returns 202 Accepted
            return
        
        # Step 4: Verify balance
        balance_response = self.client.get(
            f"/accounts/{self.account_id}/balance",
            name="GET /accounts/:id/balance"
        )
        
        # Balance verification happens in UI metrics - we just make the request
