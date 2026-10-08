# (Optional) Configure a remote backend like S3 if you want state sharing.
# terraform {
#   backend "s3" {
#     bucket = "your-tf-state-bucket"
#     key    = "event-payments/dev/terraform.tfstate"
#     region = "us-west-2"
#     dynamodb_table = "your-tf-locks"
#   }
# }
