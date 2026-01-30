$ErrorActionPreference = "Stop"
New-Item -ItemType Directory -Force -Path "bin" | Out-Null

Write-Host "Building im-ai..."
go build -o bin/im-ai.exe ./services/im-ai/cmd/im-ai

Write-Host "Building im-job..."
go build -o bin/im-job.exe ./services/im-job/cmd/im-job

Write-Host "Building im-push..."
go build -o bin/im-push.exe ./services/im-push/cmd/im-push

Write-Host "Build done."
