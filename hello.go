package main

import (
	"archive/zip"
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

const (
	AWS_S3_REGION = "ap-southeast-2"
	AWS_S3_BUCKET = "YOUR-S3-BUCKET"
	SAVE_LOCATION = "C:\\Users\User\AppData\\Roaming\\StardewValley\\Saves"
)

func zipSource(source, target string) error {
	// 1. Create a ZIP file and zip.Writer
	f, err := os.Create(target)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := zip.NewWriter(f)
	defer writer.Close()

	// 2. Go through all the files of the source
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 3. Create a local file header
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		// set compression
		header.Method = zip.Deflate

		// 4. Set relative path of a file as the header name
		header.Name, err = filepath.Rel(filepath.Dir(source), path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			header.Name += "/"
		}

		// 5. Create writer for the file header and save content of the file
		headerWriter, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = io.Copy(headerWriter, f)
		return err
	})
}

func uploadFile(session *session.Session, uploadFileDir string, s3key string) error {

	upFile, err := os.Open(uploadFileDir)
	if err != nil {
		return err
	}
	defer upFile.Close()

	upFileInfo, _ := upFile.Stat()
	var fileSize int64 = upFileInfo.Size()
	fileBuffer := make([]byte, fileSize)
	upFile.Read(fileBuffer)

	_, err = s3.New(session).PutObject(&s3.PutObjectInput{
		Bucket:               aws.String(AWS_S3_BUCKET),
		Key:                  aws.String(s3key),
		ACL:                  aws.String("private"),
		Body:                 bytes.NewReader(fileBuffer),
		ContentLength:        aws.Int64(fileSize),
		ContentType:          aws.String(http.DetectContentType(fileBuffer)),
		ContentDisposition:   aws.String("attachment"),
		ServerSideEncryption: aws.String("AES256"),
	})
	return err
}

func downloadFile(session *session.Session, filename string) error {

	// Create a file to write the S3 object to
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	downloader := s3manager.NewDownloader(session)

	numBytes, err := downloader.Download(file,
		&s3.GetObjectInput{
			Bucket: aws.String(AWS_S3_BUCKET),
			Key:    aws.String("current-save.zip"),
		})
	if err != nil {
		return err
	}

	fmt.Println("Downloaded", file.Name(), numBytes, "bytes")
	return nil
}

func getZipMD5(filename string) (string, error) {
	// Open the zip file
	zipFile, err := zip.OpenReader(filename)
	if err != nil {
		return "", err
	}
	defer zipFile.Close()

	// Compute the MD5 hash of the file contents
	hash := md5.New()
	for _, file := range zipFile.File {
		f, err := file.Open()
		if err != nil {
			return "", err
		}
		defer f.Close()

		if _, err := io.Copy(hash, f); err != nil {
			return "", err
		}
	}

	// Return the hexadecimal representation of the MD5 hash
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func main() {

	// zipping
	fmt.Println("zipping....")
	if err := zipSource(SAVE_LOCATION, "current-save.zip"); err != nil {
		log.Fatal(err)
	}

	// starting aws session
	fmt.Println("starting session....")
	session, err := session.NewSession(&aws.Config{Region: aws.String(AWS_S3_REGION)})
	if err != nil {
		log.Fatal(err)
	}

	// Download Existing Save
	fmt.Println("download existing save....")
	err = downloadFile(session, "s3-current-save.zip")
	if err != nil {
		log.Fatal(err)
	}

	// Calulate MD5
	md5, err := getZipMD5("current-save.zip")
	if err != nil {
		log.Fatal(err)
	}

	// Calulate MD5
	s3md5, err := getZipMD5("s3-current-save.zip")
	if err != nil {
		log.Fatal(err)
	}

	// Compare MD5
	fmt.Printf("MD5 of new save: %v \n", md5)
	fmt.Printf("MD5 of new save: %v \n", s3md5)

	// compare files to see if upload required.
	if s3md5 == md5 {
		fmt.Println("no change detected")
	} else {
		fmt.Println("changes detected, need to upload new file...")

		// Upload Files - REPLACE EXISTING SAVE
		fmt.Println("uploading....current-save.zip")
		err = uploadFile(session, "current-save.zip", "current-save.zip")
		if err != nil {
			log.Fatal(err)
		}
		// Upload Files - CREATE NEW POINT IN TIME SAVE
		currentTime := time.Now()
		currentTimeString := currentTime.Format("2006-01-02 15:04:05")
		new_file_name := fmt.Sprintf("current-save-%s.zip", currentTimeString)
		fmt.Printf("uploading....%v\n", new_file_name)
		err = uploadFile(session, "current-save.zip", new_file_name)
		if err != nil {
			log.Fatal(err)
		}
	}

}
