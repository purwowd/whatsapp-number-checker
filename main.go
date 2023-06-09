package main

import (
	"context"
	"fmt"
	"go.mau.fi/whatsmeow/types/events"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		fmt.Println("Received a message!", v.Message.GetConversation())
	}
}

func generateQRCode(client *whatsmeow.Client) (string, error) {
	if client.IsLoggedIn() {
		return "", fmt.Errorf("session active, please logout")
	}

	qrChan, _ := client.GetQRChannel(context.Background())
	err := client.Connect()
	if err != nil {
		return "", err
	}

	for evt := range qrChan {
		if evt.Event == "code" {
			return evt.Code, nil
		}
	}

	return "", fmt.Errorf("QR code generation failed")
}

func main() {
	dbLog := waLog.Stdout("Database", "DEBUG", true)
	container, err := sqlstore.New("sqlite3", "file:examplestore.db?_foreign_keys=on", dbLog)
	if err != nil {
		panic(err)
	}

	deviceStore, err := container.GetFirstDevice()
	if err != nil {
		panic(err)
	}

	clientLog := waLog.Stdout("Client", "DEBUG", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)
	client.AddEventHandler(eventHandler)

	router := gin.Default()

	router.GET("/qrcode", func(c *gin.Context) {
		qrCode, err := generateQRCode(client)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		qrterminal.GenerateHalfBlock(qrCode, qrterminal.L, os.Stdout)
		c.String(http.StatusOK, qrCode)
	})

	router.POST("/check-whatsapp", func(c *gin.Context) {
		var request struct {
			Phones []string `json:"phones"`
		}
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		response, err := client.IsOnWhatsApp(request.Phones)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check WhatsApp numbers"})
			return
		}

		c.JSON(http.StatusOK, response)
	})

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		client.Disconnect()
		os.Exit(0)
	}()

	if err := router.Run(":8888"); err != nil {
		panic(err)
	}
}
