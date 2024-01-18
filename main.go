package main

import (
	"context"
	"fmt"
	"log"
	"mime"
	"os"
	"os/signal"
	"strings"
	"syscall"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

func GetEventHandler(client *whatsmeow.Client) func(interface{}) {
	return func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Message:
			if v.Info.Chat.User == "status" {
			} else {
				switch {
				case v.Message.Conversation != nil:
					go ConversationMessage(v, client)
				case v.Message.ImageMessage != nil:
					go ImageMessage(v, client)
				}
			}

		}
	}
}

func ConversationMessage(evt *events.Message, client *whatsmeow.Client) {
	var messageBody = evt.Message.GetConversation()
	if messageBody == "ping" {
		client.SendMessage(context.Background(), evt.Info.Chat, &waProto.Message{
			Conversation: proto.String("pong"),
		})
	} else {
		fmt.Println("not ping :)")
		msg := &waProto.Message{
			ButtonsMessage: &waProto.ButtonsMessage{
				ContentText: proto.String("Content"),
				FooterText:  proto.String("Footer"),
				ContextInfo: &waProto.ContextInfo{},
				Buttons: []*waProto.ButtonsMessage_Button{
					{
						ButtonId:       proto.String("ButtonId"),
						ButtonText:     &waProto.ButtonsMessage_Button_ButtonText{DisplayText: proto.String("Ok")},
						Type:           waProto.ButtonsMessage_Button_RESPONSE.Enum(),
						NativeFlowInfo: &waProto.ButtonsMessage_Button_NativeFlowInfo{},
					},
				},
			},
		}

		send, err := client.SendMessage(context.Background(), evt.Info.Chat, msg)
		if err == nil {
			log.Println("Message sent (server timestamp: ", send, ")")
		} else {
			log.Fatalf("Error sending message: %v", err)
		}
	}
}

func ImageMessage(evt *events.Message, client *whatsmeow.Client) {
	sender := evt.Info.Chat.User
	// pushName := evt.Info.PushName

	img := evt.Message.GetImageMessage()
	var caption string = ""
	if evt.Message.ImageMessage.Caption != nil {
		caption = *evt.Message.ImageMessage.Caption
		fmt.Println("caption: ", caption)
	}

	if img != nil {
		file, err := client.Download(img)
		if err != nil {
			log.Printf("Failed to download image: %v", err)
			return
		}
		exts, _ := mime.ExtensionsByType(img.GetMimetype())
		// Specify the folder name you want to create
		folderName := "myFolder"
		// Create the folder at the current working directory
		err = os.MkdirAll(folderName, 0755) // 0755 is the permission mode for the folder
		if err != nil {
			fmt.Println("Error creating folder:", err)
			return
		}

		fmt.Println("Folder created:", folderName)

		fileName := fmt.Sprintf("%s/%s-%s%s", folderName, sender, evt.Info.ID, exts[0])

		if err = os.WriteFile(fileName, file, 0600); err != nil {
			log.Printf("Failed to save to server directory: %v", err)
		} else {
			log.Printf("Saved to server path at: %s", fileName)
		}

		content, err := os.ReadFile(fileName)
		if err != nil {
			fmt.Println(err)
		}
		resp, err := client.Upload(context.Background(), content, whatsmeow.MediaImage)
		if err != nil {
			fmt.Println(err)
		}

		client.SendMessage(context.Background(), evt.Info.Chat, &waProto.Message{
			Conversation: proto.String("we recieved your image with Mime type: image" + strings.Join(exts, "/")),
		})

		client.SendMessage(context.Background(), evt.Info.Chat, &waProto.Message{
			ImageMessage: &waProto.ImageMessage{
				Caption:       &caption,
				Mimetype:      proto.String("image/jpeg"),
				Url:           &resp.URL,
				DirectPath:    &resp.DirectPath,
				MediaKey:      resp.MediaKey,
				FileEncSha256: resp.FileEncSHA256,
				FileSha256:    resp.FileSHA256,
				FileLength:    &resp.FileLength,
				Height:        proto.Uint32(410),
				Width:         proto.Uint32(1200),
			},
		})
	}
}

func main() {
	dbLog := waLog.Stdout("Database", "DEBUG", true)
	// Make sure you add appropriate DB connector imports, e.g. github.com/mattn/go-sqlite3 for SQLite as we did in this minimal working example
	container, err := sqlstore.New("sqlite3", "file:examplestore.db?_foreign_keys=on", dbLog)
	if err != nil {
		panic(err)
	}

	// If you want multiple sessions, remember their JIDs and use .GetDevice(jid) or .GetAllDevices() instead.
	deviceStore, err := container.GetFirstDevice()
	if err != nil {
		panic(err)
	}

	clientLog := waLog.Stdout("Client", "INFO", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)
	client.AddEventHandler(GetEventHandler(client))

	if client.Store.ID == nil {
		// No ID stored, new login
		qrChan, _ := client.GetQRChannel(context.Background())
		err = client.Connect()
		if err != nil {
			panic(err)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				// Render the QR code here
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				// or just manually `echo 2@... | qrencode -t ansiutf8` in a terminal:
				// fmt.Println("QR code:", evt.Code)
			} else {
				fmt.Println("Login event:", evt.Event)
			}
		}
	} else {
		// Already logged in, just connect
		err = client.Connect()
		if err != nil {
			panic(err)
		}
	}

	// Listen to Ctrl+C (you can also do something else that prevents the program from exiting)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	client.Disconnect()
}
