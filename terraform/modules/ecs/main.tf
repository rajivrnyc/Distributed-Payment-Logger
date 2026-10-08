resource "aws_ecs_cluster" "this" {
  name = "${var.project_name}-ecs"
}

# Security Group for ALB (public)
resource "aws_security_group" "alb" {
  name        = "${var.project_name}-alb-sg"
  description = "Allow inbound HTTP/HTTPS"
  vpc_id      = var.vpc_id

  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = [var.alb_ingress_cidr]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

# Security Group for ECS services (private)
resource "aws_security_group" "ecs_service" {
  name        = "${var.project_name}-ecs-svc-sg"
  description = "Allow ALB to ECS traffic and ECS egress"
  vpc_id      = var.vpc_id

  ingress {
    from_port       = 8080
    to_port         = 8080
    protocol        = "tcp"
    security_groups = [aws_security_group.alb.id]
  }

  # Allow MSK Kafka traffic within the security group
  ingress {
    from_port = 9098
    to_port   = 9098
    protocol  = "tcp"
    self      = true
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

# Application Load Balancer (HTTP by default; add HTTPS later)
resource "aws_lb" "this" {
  name               = "${var.project_name}-alb"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.alb.id]
  subnets            = var.public_subnet_ids
}

resource "aws_lb_target_group" "api_tg" {
  name        = "${var.project_name}-api-tg"
  port        = 8080
  protocol    = "HTTP"
  target_type = "ip"
  vpc_id      = var.vpc_id

  health_check {
    path                = "/admin/health"
    matcher             = "200"
    healthy_threshold   = 2
    unhealthy_threshold = 2
    interval            = 10
    timeout             = 5
  }
}

resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.this.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.api_tg.arn
  }
}

output "cluster_name" { value = aws_ecs_cluster.this.name }
output "alb_dns_name" { value = aws_lb.this.dns_name }
output "alb_sg_id" { value = aws_security_group.alb.id }
output "ecs_service_sg_id" { value = aws_security_group.ecs_service.id }
output "api_target_group_arn" { value = aws_lb_target_group.api_tg.arn }
