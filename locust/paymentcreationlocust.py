from locust import HttpUser, task, between
import uuid
import random


class PaymentUser(HttpUser):
    wait_time = between(1, 3)
    
    @task(5)
    def create_payment(self):
        account_id = f"acc-{random.randint(1, 100):03d}"
        amount = random.randint(100, 10000)
        idempotency_key = str(uuid.uuid4())
        
        self.client.post(
            "/payments",
            json={
                "accountId": account_id,
                "amount": amount,
                "currency": "USD",
                "paymentMethodRef": "pm_001"
            },
            headers={
                "Idempotency-Key": idempotency_key
            },
            name="Create Payment"
        )
    
    @task(3)
    def get_payment(self):
        if not hasattr(self, 'payment_id'):
            self.payment_id = str(uuid.uuid4())
        
        self.client.get(
            f"/payments/{self.payment_id}",
            name="Get Payment"
        )
    
    @task(2)
    def get_balance(self):
        account_id = f"acc-{random.randint(1, 100):03d}"
        account_id = f"acc-{random.randint(1, 100):03d}"
        
        self.client.get(
            f"/accounts/{account_id}/balance",
            name="Get Balance"
        )
    
    def on_start(self):
        account_id = f"acc-{random.randint(1, 100):03d}"
        idempotency_key = str(uuid.uuid4())
        
        response = self.client.post(
            "/payments",
            json={
                "accountId": account_id,
                "amount": 2500,
                "currency": "USD",
                "paymentMethodRef": "pm_001"
            },
            headers={
                "Idempotency-Key": idempotency_key
            },
            name="Create Payment (Setup)"
        )
        
        if response.status_code == 202:
            data = response.json()
            self.payment_id = data.get("paymentId")
            self.account_id = account_id
