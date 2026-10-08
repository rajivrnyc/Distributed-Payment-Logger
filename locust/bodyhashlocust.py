from locust import HttpUser, task, constant
from locust.exception import StopUser
import time

class BodyHashIdempotencyUser(HttpUser):
    wait_time = constant(0)
    
    def on_start(self):
        self.user_id = self.__class__.user_count
        self.__class__.user_count += 1
        self.requests_made = 0
    
    @task
    def test_body_hash_idempotency(self):
        """
        Each user makes exactly 10 requests with identical body
        Expected with N users:
        - Total requests: N * 10
        - Successful PutItem: N
        - ConditionalCheckFailed: N * 9
        """
        if self.requests_made >= 10:
            raise StopUser()
        
        self.client.post(
            "/payments",
            json={
                "accountId": f"acc-{self.user_id:03d}",
                "amount": 1000 + self.user_id,
                "currency": "USD",
                "paymentMethodRef": "pm_test"
            },
            name="Body Hash Duplicate"
        )
        self.requests_made += 1
        time.sleep(0.1)

BodyHashIdempotencyUser.user_count = 0
