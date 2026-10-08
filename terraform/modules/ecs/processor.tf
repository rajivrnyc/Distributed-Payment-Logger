resource "aws_ecs_task_definition" "processor" {
  family                   = "${var.project_name}-processor"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = "256"
  memory                   = "512"
  execution_role_arn       = var.ecs_task_execution_role_arn
  task_role_arn            = var.ecs_task_role_arn

  container_definitions = jsonencode([
    {
      name      = "processor"
      image     = "${aws_ecr_repository.processor.repository_url}:latest"
      essential = true
      portMappings = [
        {
          containerPort = 8081
          hostPort      = 8081
          protocol      = "tcp"
        }
      ]
      environment = [
        { name = "CONSUMER_BACKEND", value = "kinesis" },
        { name = "KINESIS_STREAM_NAME", value = var.kinesis_stream_name },
        { name = "KINESIS_CHECKPOINT_TABLE", value = var.kinesis_checkpoint_table },
        { name = "CONSUMER_GROUP_ID", value = "${var.project_name}-processor" },
        { name = "AWS_REGION", value = var.aws_region },
        { name = "DDB_TABLE_IDEMPOTENCY", value = var.ddb_table_idempotency },
        { name = "DDB_TABLE_PAYMENTS", value = var.ddb_table_payments },
        { name = "DDB_TABLE_BALANCES", value = var.ddb_table_balances }
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.processor.name
          "awslogs-region"        = var.aws_region
          "awslogs-stream-prefix" = "ecs"
        }
      }
    }
  ])
}

resource "aws_ecs_service" "processor" {
  name            = "${var.project_name}-processor-service"
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.processor.arn
  desired_count   = var.processor_desired_count
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = var.private_subnet_ids
    security_groups  = [aws_security_group.ecs_service.id]
    assign_public_ip = false
  }
}

resource "aws_cloudwatch_log_group" "processor" {
  name              = "/ecs/${var.project_name}-processor"
  retention_in_days = 7

  tags = {
    Name = "${var.project_name}-processor-logs"
  }
}

resource "aws_ecr_repository" "processor" {
  name                 = "${var.project_name}-processor"
  image_tag_mutability = "MUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }

  tags = {
    Name = "${var.project_name}-processor-repo"
  }
}