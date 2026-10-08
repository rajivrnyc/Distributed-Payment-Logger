from locust import HttpUser, task, constant

user_index = 0

class IdempotencyUser(HttpUser):
    wait_time = constant(0)
    
    def on_start(self):
        global user_index
        self.user_id = user_index
        user_index += 1
        self.idem_key = f"idem-test-{self.user_id:03d}"
        self.account_id = f"acc-{self.user_id:03d}"
        self.amount = 1000 + (self.user_id * 10)
    
    @task
    def duplicate_request(self):
        self.client.post(
            "/payments",
            json={
                "accountId": self.account_id,
                "amount": self.amount,
                "currency": "USD",
                "paymentMethodRef": "pm_test"
            },
            headers={"Idempotency-Key": self.idem_key}
        )
