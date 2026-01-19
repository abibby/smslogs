package main

import (
	"embed"
	"fmt"
	"html/template"
	"maps"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	_ "github.com/joho/godotenv/autoload"
	"github.com/mergestat/timediff"
	"github.com/twilio/twilio-go"
	openapi "github.com/twilio/twilio-go/rest/api/v2010"
)

//go:embed *.html
var templates embed.FS

type Message struct {
	Body         string
	From         string
	To           string
	DateSent     time.Time
	DateSentDiff string
	Outgoing     bool
}

type Thread struct {
	PhoneNumber      string
	LastDateSent     time.Time
	LastDateSentDiff string
	Messages         []Message
}

func fromTwilio(m *openapi.ApiV2010Message, number string) Message {
	date, err := time.Parse(time.RFC1123Z, *m.DateSent)
	if err != nil {
		panic(err)
	}
	return Message{
		Body:         *m.Body,
		To:           *m.To,
		From:         *m.From,
		DateSent:     date,
		DateSentDiff: timediff.TimeDiff(date),
		Outgoing:     number == *m.From,
	}

}

func buildThreads(ogMessages []openapi.ApiV2010Message, number string) []*Thread {
	threadMap := map[string]*Thread{}
	for _, m := range ogMessages {
		phoneNumber := ""
		if *m.From == number {
			phoneNumber = *m.To
		} else if *m.To == number {
			phoneNumber = *m.From
		}
		if phoneNumber == "" {
			continue
		}

		newMessage := fromTwilio(&m, number)

		thread, ok := threadMap[phoneNumber]
		if !ok {
			threadMap[phoneNumber] = &Thread{
				PhoneNumber:      formatNumber(phoneNumber),
				LastDateSent:     newMessage.DateSent,
				LastDateSentDiff: newMessage.DateSentDiff,
				Messages:         []Message{newMessage},
			}
		} else {
			thread.Messages = append(thread.Messages, newMessage)
		}

	}

	threads := slices.Collect(maps.Values(threadMap))
	slices.SortFunc(threads, func(a, b *Thread) int {
		return b.LastDateSent.Compare(a.LastDateSent)
	})
	return threads
}

func formatNumber(number string) string {

	if len(number) != 12 || !strings.HasPrefix(number, "+1") {
		return number
	}

	return fmt.Sprintf("(%s) %s-%s", number[2:5], number[5:8], number[8:])
}

func greet(t *template.Template, client *twilio.RestClient, number string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp, err := client.Api.PageMessage(&openapi.ListMessageParams{}, "", "")
		if err != nil {
			panic(err)
		}

		err = t.ExecuteTemplate(w, "index", buildThreads(resp.Messages, number))
		if err != nil {
			fmt.Fprintf(w, "faild: %v", err)
		}
	})
}

func main() {
	accountSid := os.Getenv("TWILIO_ACCOUNT_SID")
	authToken := os.Getenv("TWILIO_AUTH_TOKEN")
	phoneNumber := os.Getenv("PHONE_NUMBER")

	client := twilio.NewRestClientWithParams(twilio.ClientParams{
		Username: accountSid,
		Password: authToken,
	})

	t, err := template.ParseFS(templates, "*")
	if err != nil {
		panic(err)
	}
	http.Handle("GET /", greet(t, client, phoneNumber))
	http.ListenAndServe(":8080", nil)
}
