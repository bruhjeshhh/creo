package main

import (
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"github.com/xuri/excelize/v2"
)

type Session struct {
	State      string
	Step       string
	RoomNumber string
	Request    string
	Complaint  string
	Name       string
	Checkin    string
	Checkout   string
	GuestCount string
	IDImageURL string
}

var sessions = make(map[string]*Session)
var mutex sync.Mutex

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Print("Error loading .env file")
	}

	http.HandleFunc("/bot", botHandler)
	fmt.Println("Bot server running on port 8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func botHandler(w http.ResponseWriter, r *http.Request) {
	from := r.FormValue("From")
	body := strings.TrimSpace(r.FormValue("Body"))
	mediaURL := r.FormValue("MediaUrl0")

	mutex.Lock()
	session, exists := sessions[from]
	if !exists {
		session = &Session{State: "menu"}
		sessions[from] = session
	}
	mutex.Unlock()

	switch session.State {
	case "menu":
		session.State = "awaiting_option"
		sendReply(w, "Welcome! Please choose an option:\n1. Room Service\n2. Housekeeping\n3. Complaint\n4. Guest Registration")
	case "awaiting_option":
		switch body {
		case "1":
			session.State = "room_service"
			sendReply(w, "Please enter your room number:")
		case "2":
			session.State = "housekeeping"
			sendReply(w, "Please enter your room number for housekeeping:")
		case "3":
			session.State = "complaint"
			sendReply(w, "Please describe your complaint:")
		case "4":
			session.State = "registering"
			session.Step = "name"
			sendReply(w, "Enter your full name:")
		default:
			sendReply(w, "Invalid option. Please choose 1-4.")
		}
	case "room_service":
		if session.RoomNumber == "" {
			session.RoomNumber = body
			sendReply(w, "What would you like to order?")
		} else {
			session.Request = body
			msg := fmt.Sprintf("Room Service Request:\nRoom: %s\nOrder: %s", session.RoomNumber, session.Request)
			sendWhatsAppToManager(msg)
			sendReply(w, "Room service request sent.")
			resetSession(from)
		}
	case "housekeeping":
		msg := fmt.Sprintf("Housekeeping Request:\nRoom: %s", body)
		sendWhatsAppToManager(msg)
		sendReply(w, "Housekeeping request sent.")
		resetSession(from)
	case "complaint":
		session.Complaint = body
		msg := fmt.Sprintf("Guest Complaint:\n%s", session.Complaint)
		sendWhatsAppToManager(msg)
		sendEmail("Guest Complaint", msg)
		sendReply(w, "Complaint sent to hotel management.")
		resetSession(from)
	case "registering":
		switch session.Step {
		case "name":
			session.Name = body
			session.Step = "checkin"
			sendReply(w, "Enter check-in date (YYYY-MM-DD):")
		case "checkin":
			session.Checkin = body
			session.Step = "checkout"
			sendReply(w, "Enter check-out date (YYYY-MM-DD):")
		case "checkout":
			session.Checkout = body
			session.Step = "guestcount"
			sendReply(w, "Enter number of guests:")
		case "guestcount":
			session.GuestCount = body
			session.Step = "idphoto"
			sendReply(w, "Please send a photo of your ID card:")
		case "idphoto":
			if mediaURL == "" {
				sendReply(w, "No photo received. Please resend your ID card image.")
				return
			}
			session.IDImageURL = mediaURL
			saveToExcel(session)
			msg := fmt.Sprintf("New Guest Registration:\nName: %s\nCheck-in: %s\nCheck-out: %s\nGuests: %s\nID: %s", session.Name, session.Checkin, session.Checkout, session.GuestCount, session.IDImageURL)
			sendEmail("Guest Registration", msg)
			sendReply(w, "Registration completed. Thank you!")
			resetSession(from)
		}
	default:
		session.State = "menu"
		sendReply(w, "Something went wrong. Restarting session.")
	}
}

func sendReply(w http.ResponseWriter, msg string) {
	fmt.Fprint(w, msg)
}

func sendWhatsAppToManager(msg string) {
	// Optional: use curl from Go to save Twilio message quota
	fmt.Printf("[WHATSAPP] Message to manager: %s\n", msg)
}

func sendEmail(subject, body string) {
	email := os.Getenv("EMAIL_USERNAME")
	pass := os.Getenv("EMAIL_PASSWORD")
	recv := os.Getenv("MANAGER_EMAIL")
	smtpHost := "smtp.gmail.com"
	smtpPort := "587"
	msg := []byte("Subject: " + subject + "\r\n\r\n" + body)
	auth := smtp.PlainAuth("", email, pass, smtpHost)
	smtp.SendMail(smtpHost+":"+smtpPort, auth, email, []string{recv}, msg)
}

func saveToExcel(s *Session) {
	filename := "guests.xlsx"
	var f *excelize.File
	var err error

	if _, err = os.Stat(filename); os.IsNotExist(err) {
		f = excelize.NewFile()
		f.SetSheetRow("Sheet1", "A1", &[]string{"Name", "Check-in", "Check-out", "Guests", "Time"})
	} else {
		f, err = excelize.OpenFile(filename)
		if err != nil {
			log.Println("Error opening Excel file:", err)
			return
		}
	}

	timeNow := time.Now().Format("2006-01-02 15:04")
	rows, _ := f.GetRows("Sheet1")
	rowIndex := len(rows) + 1
	cell := fmt.Sprintf("A%d", rowIndex)
	f.SetSheetRow("Sheet1", cell, &[]interface{}{s.Name, s.Checkin, s.Checkout, s.GuestCount, timeNow})
	f.SaveAs(filename)
}

func resetSession(from string) {
	mutex.Lock()
	defer mutex.Unlock()
	delete(sessions, from)
}
