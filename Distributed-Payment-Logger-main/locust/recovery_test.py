from locust import HttpUser, task, constant_pacing, events
import random

class PaymentUser(HttpUser):
    wait_time = constant_pacing(0.005)  # 200 req/sec = 1 req per 5ms per user
    
    @task
    def create_payment(self):
        account_id = f"acc-{random.randint(1, 100)}"
        payload = {
            "accountId": account_id,
            "amount": random.randint(1000, 50000),
            "currency": "USD",
            "paymentMethodRef": "pm_test_card"
        }
        headers = {
            "Content-Type": "application/json",
            "Idempotency-Key": f"recovery-test-{random.randint(1, 10000000)}"
        }
        
        with self.client.post("/payments", json=payload, headers=headers, catch_response=True) as response:
            if response.status_code == 200 or response.status_code == 202:
                response.success()
            else:
                response.failure(f"Failed: {response.status_code}")
