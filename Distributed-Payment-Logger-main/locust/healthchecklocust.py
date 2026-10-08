from locust import HttpUser, task, constant, events
import logging

logger = logging.getLogger(__name__)


class HealthCheckUser(HttpUser):
    wait_time = constant(1)
    @task
    def check_health(self):
        self.client.get(
            "/admin/health",
            name="Health Check"
        )