package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/twilio/twilio-go"
	openapi "github.com/twilio/twilio-go/rest/api/v2010"
)

type UserStore struct {
	Numbers map[string]bool `json:"numbers"`
	Mutex   sync.Mutex
}

var userStore = &UserStore{Numbers: make(map[string]bool)}

func init() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found — using environment variables from Render")
	}

	loadUsers()
}

func loadUsers() {
	file, err := os.ReadFile("users.json")
	if err == nil {
		json.Unmarshal(file, &userStore.Numbers)
	}
}

func saveUsers() {
	userStore.Mutex.Lock()
	defer userStore.Mutex.Unlock()
	file, _ := json.MarshalIndent(userStore.Numbers, "", "  ")
	os.WriteFile("users.json", file, 0644)
}

func whatsappWebhook(c *gin.Context) {
	incomingMsg := strings.ToLower(c.PostForm("Body"))
	from := c.PostForm("From")

	log.Println("Incoming From:", from)
	log.Println("Incoming Message:", incomingMsg)

	userStore.Mutex.Lock()
	userStore.Numbers[from] = true
	userStore.Mutex.Unlock()
	saveUsers()

	var reply string
	switch incomingMsg {
	case "menu", "1":
		reply = "Here is the menu: https://your-menu-url.com/menu.pdf"
	case "reservation", "2":
		reply = "Please send your reservation in this format:\nName, Date, Time, Guests"
	case "promotion", "3":
		reply = "🔥 Special Offer: 20% off this week on all orders above ₹500!"
	default:
		if strings.Count(incomingMsg, ",") >= 3 {
			// Assume it's a reservation request
			sendReservationEmail(
				os.Getenv("EMAIL_SENDER"),
				os.Getenv("EMAIL_PASSWORD"),
				os.Getenv("RESTAURANT_EMAIL"),
				from,
				incomingMsg,
			)
			reply = "✅ Reservation received! We'll get back to you shortly.\n\nYou'll also receive a feedback form after your reservation."
		} else {
			reply = "Welcome to Demo Restaurant! 🍽️\n\n1. Menu\n2. Reservation\n3. Promotions\n\nReply with a number or keyword."
		}
	}

	err := sendWhatsAppMessage(from, reply)
	if err != nil {
		log.Println("❌ Error sending WhatsApp message:", err)
	} else {
		log.Println("✅ WhatsApp message sent to", from)
	}

	c.Status(http.StatusOK) // Prevents returning "ok" as message
}

func sendReservationEmail(fromEmail, password, to, customerNumber, details string) {
	subject := "New Reservation Request"
	body := fmt.Sprintf("Customer: %s\nDetails: %s", customerNumber, details)
	msg := fmt.Sprintf("Subject: %s\n\n%s", subject, body)

	auth := smtp.PlainAuth("", fromEmail, password, "smtp.gmail.com")
	err := smtp.SendMail("smtp.gmail.com:587", auth, fromEmail, []string{to}, []byte(msg))
	if err != nil {
		log.Println("❌ Error sending email:", err)
	} else {
		log.Println("📧 Reservation email sent.")
	}
}

func sendWhatsAppMessage(to, message string) error {
	accountSid := os.Getenv("TWILIO_ACCOUNT_SID")
	authToken := os.Getenv("TWILIO_AUTH_TOKEN")
	sender := os.Getenv("TWILIO_WHATSAPP_NUMBER")

	client := twilio.NewRestClientWithParams(twilio.ClientParams{
		Username: accountSid,
		Password: authToken,
	})

	params := &openapi.CreateMessageParams{}
	params.SetTo(to)
	params.SetFrom(sender)
	params.SetBody(message)

	resp, err := client.Api.CreateMessage(params)
	if err != nil {
		return err
	}
	log.Printf("✅ Message sent with SID: %s", *resp.Sid)
	return nil
}

func broadcastPromotion(message string) {
	for number := range userStore.Numbers {
		err := sendWhatsAppMessage(number, message)
		if err != nil {
			log.Println("❌ Error broadcasting to", number, ":", err)
		}
	}
}

func main() {
	r := gin.Default()
	r.POST("/whatsapp", whatsappWebhook)

	r.GET("/broadcast", func(c *gin.Context) {
		msg := c.Query("msg")
		if msg == "" {
			c.String(http.StatusBadRequest, "Missing message")
			return
		}
		broadcastPromotion(msg)
		c.String(http.StatusOK, "Broadcast sent")
	})

	r.Run(":10000")
}
