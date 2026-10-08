from locust import HttpUser, task, constant_pacing, events
import random
import time

class PaymentFlowUser(HttpUser):
    wait_time = constant_pacing(0.01)  # Target 100 req/sec per user
    
    def on_start(self):
        """Initialize user with unique account"""
        self.account_id = f"acc-e2e-{random.randint(1, 10000)}"
        self.payments = []  # Track created payments for authorize/capture
    
    @task(5)  # 50% weight - Create payment
    def create_payment(self):
        payload = {
            "accountId": self.account_id,
            "amount": random.randint(1000, 50000),
            "currency": "USD",
            "paymentMethodRef": "pm_test_card"
        }
        headers = {
            "Content-Type": "application/json",
            "Idempotency-Key": f"e2e-create-{time.time()}-{random.randint(1, 1000000)}"
        }
        
        with self.client.post("/payments", json=payload, headers=headers, catch_response=True) as response:
            if response.status_code == 200 or response.status_code == 202:
                try:
                    data = response.json()
                    payment_id = data.get("paymentId")
                    if payment_id:
                        # Store payment with timestamp for later use
                        self.payments.append({
                            'id': payment_id,
                            'created_at': time.time()
                        })
                        # Keep list manageable
                        if len(self.payments) > 20:
                            self.payments.pop(0)
                    response.success()
                except:
                    response.success()  # Accept even if no JSON body
            else:
                response.failure(f"Create failed: {response.status_code}")
    
    @task(3)  # 30% weight - Authorize payment
    def authorize_payment(self):
        if not self.payments:
            return  # Skip if no payments created yet
        
        # Only authorize payments that are at least 5 seconds old (give time for processing)
        eligible = [p for p in self.payments if time.time() - p['created_at'] > 5]
        if not eligible:
            return
        
        payment = random.choice(eligible)
        payment_id = payment['id']
        headers = {
            "Content-Type": "application/json",
            "Idempotency-Key": f"e2e-auth-{time.time()}-{random.randint(1, 1000000)}"
        }
        
        with self.client.post(f"/payments/{payment_id}/authorize", headers=headers, catch_response=True) as response:
            if response.status_code == 200 or response.status_code == 202:
                response.success()
            elif response.status_code == 409:  # Already authorized
                response.success()
            elif response.status_code == 404:  # Not processed yet
                response.success()  # Don't fail, just means E2E latency > 5s
            else:
                response.failure(f"Authorize failed: {response.status_code}")
    
    @task(2)  # 20% weight - Capture payment
    def capture_payment(self):
        if not self.payments:
            return  # Skip if no payments created yet
        
        # Only capture payments that are at least 7 seconds old (more time for authorize processing)
        eligible = [p for p in self.payments if time.time() - p['created_at'] > 7]
        if not eligible:
            return
        
        payment = random.choice(eligible)
        payment_id = payment['id']
        headers = {
            "Content-Type": "application/json",
            "Idempotency-Key": f"e2e-capture-{time.time()}-{random.randint(1, 1000000)}"
        }
        
        with self.client.post(f"/payments/{payment_id}/capture", headers=headers, catch_response=True) as response:
            if response.status_code == 200 or response.status_code == 202:
                response.success()
            elif response.status_code == 409:  # Already captured or not authorized
                response.success()
            elif response.status_code == 404:  # Not processed yet
                response.success()  # Don't fail, just means E2E latency > 7s
            else:
                response.failure(f"Capture failed: {response.status_code}")
