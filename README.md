# serverless

This repository contains code for creating a lambda function in aws. The lambda function performs the following tasks.

1. Download the release in given URL
2. If it is a valid download, uploads it to gcp bucket
3. Sends the status of download as an email with necessary details to the user.
4. Add an entry to track the emails sent in DynamoDB

The lambda function has 4 environment variables listed below

1. GCPKEY - key to connect to gcp
2. GCBUCKET - bucket name in gcp cloud where the release should be uploaded
3. DYNAMOTB - table name of dynmodb
4. MANDRILLKEY - the key needed for sending email using mandrill of mailchimp platform

The steps to create the zip of this lambda function is given below:

Step 1: Set aws profile

`set AWS_PROFILE=<profilename>`

Step 2: Set env for build

`set GOOS=linux`

`set GOARCH=amd64`

`set CGO_ENABLED=0`

Step 3: Build the code

`go build main.go`

Step 4: Create the zip file

`%USERPROFILE%\Go\bin\build-lambda-zip.exe -o myFunction.zip main`

The zip file can be used to deploy lambda using any IaC tool.