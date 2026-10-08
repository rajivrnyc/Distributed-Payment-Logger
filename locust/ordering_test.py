from locust import HttpUser, task, between, events
from locust.exception import StopUser
import time
import threading

class OrderingTestUser(HttpUser):
    wait_time = between(0.1, 0.2)
    user_counter = 0
    counter_lock = threading.Lock()
    
    def on_start(self):
        with OrderingTestUser.counter_lock:
            OrderingTestUser.user_counter += 1
            self.account_id = f"acc-ordering-{OrderingTestUser.user_counter}"
        self.payment_id = None
        self.step = 0
    
    @task
    def payment_flow(self):
        if self.step == 0:
            self.create_payment()
        elif self.step == 1:
            time.sleep(0.5)
            self.authorize_payment()
        elif self.step == 2:
            time.sleep(0.5)
            self.capture_payment()
        else:
            raise StopUser()
    
    def create_payment(self):
        payload = {
            "accountId": self.account_id,
            "amount": 10000,
            "currency": "USD",
            "paymentMethodRef": "pm_test"
        }
        headers = {
            "Content-Type": "application/json",
            "Idempotency-Key": f"ordering-{self.account_id}-create"
        }
        
        with self.client.post("/payments", json=payload, headers=headers, catch_response=True) as response:
            if response.status_code in [200, 202]:
                self.payment_id = response.json().get("paymentId")
                self.step = 1
                response.success()
            else:
                response.failure(f"Create failed: {response.status_code}")
    
    def authorize_payment(self):
        if not self.payment_id:
            return
        
        headers = {"Content-Type": "application/json"}
        with self.client.post(f"/payments/{self.payment_id}/authorize", json={}, headers=headers, catch_response=True) as response:
            if response.status_code in [200, 202, 204]:
                self.step = 2
                response.success()
            else:
                response.failure(f"Authorize failed: {response.status_code}")
    
    def capture_payment(self):
        if not self.payment_id:
            return
        
        payload = {"amount": 10000}
        headers = {"Content-Type": "application/json"}
        with self.client.post(f"/payments/{self.payment_id}/capture", json=payload, headers=headers, catch_response=True) as response:
            if response.status_code in [200, 202, 204]:
                self.step = 3
                response.success()
            else:
                response.failure(f"Capture failed: {response.status_code}")

@events.test_start.add_listener
def on_test_start(environment, **kwargs):
    OrderingTestUser.user_counter = 0
