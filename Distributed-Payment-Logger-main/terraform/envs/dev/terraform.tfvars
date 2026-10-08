project_name = "event-payments"
aws_region   = "us-west-2"

enable_msk           = true            # set to false if you don’t have MSK
kafka_payments_topic = "payments.events"

# When enable_msk = false, you must set this:
# kafka_bootstrap_servers = "broker1:9092,broker2:9092"

# Use your account's AZ names
azs = ["us-west-2a", "us-west-2b", "us-west-2c"]

vpc_cidr             = "10.20.0.0/16"
public_subnet_cidrs  = ["10.20.0.0/24", "10.20.1.0/24", "10.20.2.0/24"]
private_subnet_cidrs = ["10.20.10.0/24", "10.20.11.0/24", "10.20.12.0/24"]

one_nat_gateway = true

# For a locked-down demo, put your public IP in CIDR form, e.g., "203.0.113.42/32"
alb_ingress_cidr = "0.0.0.0/0"

