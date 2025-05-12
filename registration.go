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
		session = &Session{State: "language_selection"}
		sessions[from] = session
	}
	mutex.Unlock()

	if strings.EqualFold(body, "menu") {
		resetSession(from)
		session = &Session{State: "language_selection"}
		sessions[from] = session
		sendReply(w, "🙏 Welcome to Kamdhenu Sadan – A Sacred Stay in the Heart of Devbhoomi\nYour peace and comfort are our blessings to serve.\nPlease select your preferred language to begin your journey with us:\n1️⃣ हिंदी\n2️⃣ English")
		return
	}

	switch session.State {
	case "language_selection":
		switch body {
		case "1":
			session.State = "unsupported_language"
			sendReply(w, "क्षमा करें, हिंदी संस्करण जल्द ही उपलब्ध होगा। कृपया English चुनें।")
		case "2":
			session.State = "main_menu"
			sendReply(w, "How can I assist you today?\n1️⃣ Registration\n2️⃣ Room Service")
		default:
			sendReply(w, "Please select a valid language option:\n1️⃣ हिंदी\n2️⃣ English")
		}

	case "main_menu":
		switch body {
		case "1":
			session.State = "registering"
			session.Step = "name"
			sendReply(w, "📝 Let’s get you checked in. Please provide the following details:\n* Full Name:")
		case "2":
			session.State = "room_service_menu"
			sendReply(w, "🛎️ How can we help you in your room? Please choose an option:\n1️⃣ Order Food\n2️⃣ Request Housekeeping\n3️⃣ Report an Issue / Complaint")
		default:
			sendReply(w, "Please choose a valid option:\n1️⃣ Registration\n2️⃣ Room Service")
		}

	case "room_service_menu":
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
		default:
			sendReply(w, "Invalid option. Please choose 1-3.")
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
			sendReply(w, "* Number of Guests:")
		case "checkin":
			session.GuestCount = body
			session.Step = "checkout"
			sendReply(w, "* Check-In Date (YYYY-MM-DD):")
		case "checkout":
			session.Checkin = body
			session.Step = "guestcount"
			sendReply(w, "* Check-Out Date (YYYY-MM-DD):")
		case "guestcount":
			session.Checkout = body
			session.Step = "idphoto"
			sendReply(w, "📎 Kindly upload a valid Government-issued ID (Aadhar, PAN, etc.):")
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
		session.State = "language_selection"
		sendReply(w, "Something went wrong. Restarting session. Please select language again:\n1️⃣ हिंदी\n2️⃣ English")
	}
}

func sendReply(w http.ResponseWriter, msg string) {
	fmt.Fprint(w, msg)
}

func sendWhatsAppToManager(msg string) {
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
