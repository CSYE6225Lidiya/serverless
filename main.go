package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	m "github.com/keighl/mandrill"
	"google.golang.org/api/option"
)

type MessageM struct {
	Name           string `json:"name"`
	AssignmentName string `json:"assName"`
	RetryNo        string `json:"retry"`
	Email          string `json:"email"`
	SubTime        string `json:"time"`
	DownloadURL    string `json:"downloadURL"`
}

type Cred struct {
	Type             string `json:"type"`
	ProjID           string `json:"project_id"`
	PvtKeyId         string `json:"private_key_id"`
	PvtKey           string `json:"private_key"`
	ClientEMail      string `json:"client_email"`
	ClientId         string `json:"client_id"`
	AuthURL          string `json:"auth_uri"`
	TokenURL         string `json:"token_uri"`
	AuthProviderCert string `json:"auth_provider_x509_cert_url"`
	ClientCertURL    string `json:"client_x509_cert_url"`
	UniDom           string `json:"universe_domain"`
}

type Item struct {
	ID             string
	Name           string
	Email          string
	DownloadStatus string
	UploadPath     string
}

func handler(ctx context.Context, snsEvent events.SNSEvent) {

	var emailRecipient string
	var downloadURL string

	var user string
	var assignment string
	var retry string
	var downloadStatus string
	var objectName string
	var filePath string
	var bucketName string = os.Getenv("GCBUCKET")
	var uploadPath string

	for _, record := range snsEvent.Records {
		snsRecord := record.SNS
		fmt.Printf("[%s %s] Message = %s \n", record.EventSource, snsRecord.Timestamp, snsRecord.Message)
		log.Printf("SNS EventSourse:%s", record.EventSource)
		log.Printf("SNS TimeS: %s", snsRecord.Timestamp)
		log.Printf("SNS Msg: %s", snsRecord.Message)

		var snsMessage MessageM
		if err := json.Unmarshal([]byte(snsRecord.Message), &snsMessage); err != nil {
			log.Printf("Error unmarshalling SNS message: %v", err)

		}

		//use the myEvent struct for further processing
		log.Printf("Name from SNS Message: %s", snsMessage.Name)
		log.Printf("AssignmentName from SNS Message: %s", snsMessage.AssignmentName)
		log.Printf("Email from SNS Message: %s", snsMessage.Email)
		log.Printf("RetryNo from SNS Message: %s", snsMessage.RetryNo)
		log.Printf("SubTime from SNS Message: %s", snsMessage.SubTime)
		log.Printf("DownloadURL from SNS Message: %s", snsMessage.DownloadURL)

		emailRecipient = snsMessage.Email
		downloadURL = snsMessage.DownloadURL
		user = snsMessage.Name
		assignment = snsMessage.AssignmentName
		retry = snsMessage.RetryNo

	}

	log.Println("Gonna Download Artifact")
	log.Println("Tne email recipient is ", emailRecipient)

	// Specify the local file path to save the downloaded ZIP file
	zipFilePath := "/tmp/downloaded_release.zip"

	// Download the release ZIP file
	err := downloadFile(downloadURL, zipFilePath)
	if err != nil {
		log.Println("Error downloading release file:", err.Error())
		downloadStatus = "Unable to download - Not a valid submission"
		uploadPath = "N/A"
		//	return
	} else {
		downloadStatus = "Successfully Downloaded"
		var gcpKey = os.Getenv("GCPKEY")
		log.Printf("GCP Key Val :%s", gcpKey)
		objectName = user + "/" + assignment + "/" + retry + "/" + "downloaded_release.zip"
		filePath = "/tmp/downloaded_release.zip"

		err = uploadFileToGCS(bucketName, objectName, filePath, gcpKey)
		if err != nil {
			log.Printf("Error uploading file to GCS: %v\n", err.Error())
		} else {
			uploadPath = bucketName + "/" + objectName
		}
	}

	// Email Using MailChimp
	mandrillKey := os.Getenv("MANDRILLKEY")
	client := m.ClientWithKey(mandrillKey)

	message := &m.Message{}
	message.AddRecipient(emailRecipient, user, "to")
	message.FromEmail = "sender@demo.lidiyacloud.me"
	message.FromName = "Lidiya's Demo Domain - Assignment Submission Status"
	message.Subject = "Submission Status Update"
	message.HTML = fmt.Sprintf("<h1>Status of your latest submission</h1><p>Hello %s</p><p>Assignment Name: %s</p><p>Download Status: %s</p><p>FilePath in GCP: %s</p>", user, assignment, downloadStatus, uploadPath)
	message.Text = "You won!!Wohooooo" //Optional content to be sent
	responses, err := client.MessagesSend(message)
	fmt.Println("ERRR:", err)
	fmt.Println("RESPSTATUS", responses)

	if err != nil {
		log.Println("Error Sending email:", err.Error())
	} else { //Add an entry to dynamodb
		log.Println("Logging to dynamo db")
		sess := session.Must(session.NewSession(&aws.Config{
			Region: aws.String("us-east-1"), // replace with your AWS region
		}))
		// Create DynamoDB client
		svc := dynamodb.New(sess)
		randomString := generateRandomString(5)

		item := Item{
			ID:             randomString,
			Name:           user,
			Email:          emailRecipient,
			DownloadStatus: downloadStatus,
			UploadPath:     uploadPath,
		}
		av, err := dynamodbattribute.MarshalMap(item)
		if err != nil {
			log.Fatalf("Got error marshalling new dynamodb item: %s", err)
		}

		var dynamoTable string = os.Getenv("DYNAMOTB")

		input := &dynamodb.PutItemInput{
			Item:      av,
			TableName: aws.String(dynamoTable),
		}
		_, err = svc.PutItem(input)
		if err != nil {
			log.Fatalf("Got error calling PutItem: %s", err)
		}

		fmt.Println("Successfully added '" + item.Name + "' (" + item.Email + ") to table " + dynamoTable)
	}

}

func downloadFile(url, filePath string) error {
	if !strings.HasSuffix(url, ".zip") {
		return fmt.Errorf("not a valid zip:%s", url)
	}

	response, err := http.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP request failed with status code %d", response.StatusCode)
	}

	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, response.Body)
	return err
}

func uploadFileToGCS(bucketName, objectName, filePath, gcpKey string) error {
	// Set up Google Cloud Storage client
	ctx := context.Background()

	servAccKey := gcpKey

	// Decode the base64-encoded key
	decodedKey, err := base64.StdEncoding.DecodeString(servAccKey)
	if err != nil {
		log.Println("Error decoding base64:", err.Error())
	}

	var serviceAccount Cred

	// Convert the byte slice to a string
	jsonString := string(decodedKey)
	err = json.Unmarshal([]byte(jsonString), &serviceAccount)
	if err != nil {
		log.Println("Error parsing JSON GCP Key:", err.Error())
	}

	log.Println("Client Email:", serviceAccount.ClientEMail)
	log.Println("Private Key:", serviceAccount.PvtKey)
	client, err := storage.NewClient(ctx, option.WithCredentialsJSON([]byte(jsonString)))

	if err != nil {
		log.Printf("Error Creating GCP Client: %v ", err.Error())
		return fmt.Errorf(" GCS CLIENT ERR: %v", err)
	}
	defer client.Close()

	// Open the local file
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("FILEOPENERR: %v", err)
	}
	defer file.Close()

	// Create a GCS client and bucket handle
	bucket := client.Bucket(bucketName)

	// Create a new GCS object and upload it
	obj := bucket.Object(objectName)
	wc := obj.NewWriter(ctx)

	// Copy the file contents to the GCS object
	if _, err := io.Copy(wc, file); err != nil {
		log.Printf("Error in copy to GCS bucket:%v", err.Error())
		return fmt.Errorf("ERR GCS COPY: %v", err)
	}

	// Close the GCS writer to flush the data to GCS
	if err := wc.Close(); err != nil {
		log.Printf("Error closing GCS bucket:%v", err.Error())
		return fmt.Errorf("ERR GCS WRITER CLOSE: %v", err)
	}

	log.Printf("File %s uploaded to gs://%s/%s\n", filePath, bucketName, objectName)
	log.Print("Successfully uploaded to gcs bucket")
	return nil
}

func generateRandomString(length int) string {
	rand.Seed(time.Now().UnixNano())

	// Define the characters that can be used in the random string
	chars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	// Create a slice to store the random characters
	result := make([]byte, length)

	// Generate the random string
	for i := 0; i < length; i++ {
		result[i] = chars[rand.Intn(len(chars))]
	}

	// Convert the byte slice to a string and return
	return string(result)
}

func main() {
	lambda.Start(handler)
}
