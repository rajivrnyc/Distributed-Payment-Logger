from locust import FastHttpUser, task, between
import uuid
import json

class PaymentUser(FastHttpUser):
    wait_time = between(1, 3)  # simulate user think time

    def on_start(self):
        # Called when a simulated user starts
        self.account_id = "acc-001"
        self.payment_id = None

    @task(3)  # Give higher weight to creating payments
    def create_payment(self):
        idem_key = str(uuid.uuid4())  # unique idempotency key for each request
        payload = {
            "accountId": self.account_id,
            "amount": 1000,
            "currency": "USD",
            "paymentMethodRef": "pm-test-001"  
        }
        headers = {
            "Content-Type": "application/json",
            "Idempotency-Key": idem_key
        }
        with self.client.post("/payments", data=json.dumps(payload), headers=headers, catch_response=True) as response:
            if response.status_code == 202:
                # Save the payment ID returned for later get
                try:
                    self.payment_id = response.json().get("paymentId")
                    response.success()
                except Exception:
                    response.failure("Failed to parse paymentId")
            else:
                response.failure(f"Unexpected status code {response.status_code}")

    @task(2)
    def get_payment(self):
        if self.payment_id:
            self.client.get(f"/payments/{self.payment_id}")

    @task(1)
    def get_balance(self):
        self.client.get(f"/accounts/{self.account_id}/balance")
