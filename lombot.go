package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/steambap/captcha"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"time"

	tb "gopkg.in/tucnak/telebot.v2"
)

var (
	pwd       string
	retryPath string
	token     *string
	wait      *int64
)

type Credentials struct {
	User   *tb.User
	Key    string
	Pesans []*tb.Message
	ch     chan struct{}
}

type MyBot struct {
	Bot      *tb.Bot
	UserJoin map[int]*Credentials
	retry    map[int]int
}

func init() {
	pwd, _ = os.Getwd()
	retryPath = pwd + "/retry.json"
	token = flag.String("t", "", "token bot telegram")
	wait = flag.Int64("w", 5, "lama menunggu jawaban")

	flag.Parse()
}

func main() {
	if *token == "" {
		log.Fatal("Token harus diisi : -t <token>")
	}

	if _, err := os.Stat(retryPath); errors.Is(err, os.ErrNotExist) {
		err := writeFile(retryPath, []byte(""))
		if err != nil {
			log.Panicf("error to write file : %v", err.Error())
		}
	}

	b, err := tb.NewBot(tb.Settings{
		// You can also set custom API URL.
		// If field is empty it equals to "https://api.telegram.org".
		// URL: "http://195.129.111.17:8012",

		Token:  *token,
		Poller: &tb.LongPoller{Timeout: 10 * time.Second},

		// sync true to ensure all messages deleted properly
		Synchronous: true,
	})

	if err != nil {
		log.Fatal("token salah: " + err.Error())
		return
	}

	defer b.Stop()

	myBot := &MyBot{
		Bot:      b,
		retry:    make(map[int]int),
		UserJoin: make(map[int]*Credentials),
	}

	fileRetry, err := os.ReadFile(retryPath)
	if err != nil {
		log.Fatal("failed to read file retry:", err.Error())
		return
	}

	if len(fileRetry) > 0 {
		err := json.Unmarshal(fileRetry, &myBot.retry)
		if err != nil {
			log.Fatal("failed to unmarshal retry:", err.Error())
		}
	}

	b.Handle("/halo", func(m *tb.Message) {
		if !m.FromGroup() {
			return
		}
		b.Send(m.Chat, fmt.Sprintf("Halo %v %v", m.Sender.FirstName, m.Sender.LastName))
	})

	b.Handle("/id", func(m *tb.Message) {
		// all the text messages that weren't
		// captured by existing handlers
		if !m.FromGroup() {
			return
		}
		msg := fmt.Sprintf("%s %s, ID Anda adalah %d", m.Sender.FirstName, m.Sender.LastName, m.Sender.ID)
		b.Send(m.Chat, msg)
	})

	b.Handle("/testpoll", func(m *tb.Message) {
		if !m.FromGroup() {
			return
		}
		poll := &tb.Poll{
			Type:          tb.PollQuiz,
			Question:      "Test Poll",
			CloseUnixdate: time.Now().Unix() + 60,
			Explanation:   "Explanation",
			Options: []tb.PollOption{
				{Text: "1"},
				{Text: "2"},
				{Text: "3"},
			},
			CorrectOption: 2,
		}

		b.Send(m.Chat, poll)
	})

	b.Handle(tb.OnUserJoined, func(m *tb.Message) {
		if !m.FromGroup() {
			return
		}

		cm, err2 := b.ChatMemberOf(m.Chat, m.Sender)
		if err2 != nil {
			return
		}

		if cm.Role == "administrator" || cm.Role == "creator" {
			b.Delete(m)
			msg := fmt.Sprintf("Selamat datang %v %v", m.UserJoined.FirstName, m.UserJoined.LastName)
			b.Send(m.Chat, msg)
			return
		}

		myBot.retry[m.UserJoined.ID]++
		saveJson(myBot.retry, retryPath)

		cm2, err2 := b.ChatMemberOf(m.Chat, m.UserJoined)
		if err2 != nil {
			return
		}
		cm2.RestrictedUntil = time.Now().Add(time.Duration(myBot.retry[m.UserJoined.ID]*5) * time.Minute).Unix()
		cm2.CanSendMessages = true
		err := b.Restrict(m.Chat, cm2)
		if err != nil {
			return
		}

		img, err := captcha.New(300, 100, func(o *captcha.Options) {
			o.Noise = 3
			o.CurveNumber = 13
		})
		credential := &Credentials{
			User:   m.UserJoined,
			Key:    img.Text,
			Pesans: make([]*tb.Message, 0),
			ch:     make(chan struct{}),
		}

		pwd, _ := os.Getwd()
		file, err := os.Create(pwd + "/c.png")
		if err != nil {
			panic("failed to open c.png")
		}
		defer func() {
			file.Close()
		}()

		err = img.WriteImage(file)
		if err != nil {
			panic("failed to write img")
		}
		cpt := &tb.Photo{
			File: tb.FromDisk(pwd + "/c.png"),
		}

		minfo := fmt.Sprintf(`
Hai %v %v..!
Tulis captcha dalam waktu %v menit.
Huruf besar dan kecil berpengaruh`, m.UserJoined.FirstName, m.UserJoined.LastName, *wait)

		b.Delete(m)
		info, err := b.Send(m.Chat, minfo)
		cmsg, err := b.Send(m.Chat, cpt)
		credential.Pesans = append(credential.Pesans, info)
		credential.Pesans = append(credential.Pesans, cmsg)
		if err != nil {
			fmt.Println("failed to send msg :", err.Error())
			return
		}
		myBot.UserJoin[m.UserJoined.ID] = credential

		// will race condition, but it's ok because we select it by sender id
		go myBot.acceptOrDelete(m, cm2)
	})

	b.Handle(tb.OnText, func(m *tb.Message) {
		cred := myBot.UserJoin[m.Sender.ID]
		if isNewUser(cred) {
			if m.Text == cred.Key {
				b.Delete(m)
				cred.ch <- struct{}{}
				return
			}
			b.Delete(m)
			if len(cred.Pesans) < 2 {
				send, _ := b.Send(m.Chat, "Anda salah memasukkan kode.\nHuruf besar dan kecil berpengaruh")
				cred.Pesans = append(cred.Pesans, send)
			} else {
				b.Delete(m)
			}
		}
	})

	b.Handle(tb.OnUserLeft, func(m *tb.Message) {
		if !m.FromGroup() {
			return
		}
		b.Delete(m)
	})

	b.Handle(tb.OnPhoto, myBot.notText())
	b.Handle(tb.OnAnimation, myBot.notText())
	b.Handle(tb.OnVoice, myBot.notText())
	b.Handle(tb.OnVideo, myBot.notText())
	b.Handle(tb.OnVideoNote, myBot.notText())
	b.Handle(tb.OnDice, myBot.notText())
	b.Handle(tb.OnDocument, myBot.notText())
	b.Handle(tb.OnContact, myBot.notText())
	b.Handle(tb.OnSticker, myBot.notText())
	b.Handle(tb.OnLocation, myBot.notText())

	fmt.Println("bot started")
	b.Start()

}

func (myBot *MyBot) acceptOrDelete(m *tb.Message, cm *tb.ChatMember) {
	cred := myBot.UserJoin[m.UserJoined.ID]
	select {
	case <-time.After(time.Duration(*wait) * time.Minute):
		// restrict := time.Now().Add(time.Duration(myBot.retry[m.Sender.ID]*5) * time.Minute).Unix()

		err := myBot.Bot.Ban(m.Chat, cm, true)
		if err != nil {
			fmt.Println("failed to ban user:", err.Error())
		}

		cred.deleteMessages(myBot.Bot)
		return

	case <-cred.ch:
		cred.deleteMessages(myBot.Bot)
		delete(myBot.UserJoin, m.UserJoined.ID)

		msg := fmt.Sprintf("Selamat datang %v %v", cred.User.FirstName, cred.User.LastName)

		delete(myBot.retry, m.UserJoined.ID)
		saveJson(myBot.retry, retryPath)

		myBot.Bot.Send(m.Chat, msg)
		return
	}
}

func saveJson(data interface{}, path string) {
	b1, err := json.Marshal(data)
	if err != nil {
		log.Panicf("failed to marshal : %v", err.Error())
	}

	if err := writeFile(path, b1); err != nil {
		log.Panicf("failed to write file : %v", err.Error())
	}
}

func (myBot *MyBot) notText() func(m *tb.Message) {
	return func(m *tb.Message) {
		if !m.FromGroup() {
			return
		}

		cred := myBot.UserJoin[m.Sender.ID]
		if isNewUser(cred) {
			myBot.Bot.Delete(m)
		}
	}
}

func isNewUser(cred *Credentials) bool {
	return cred != nil && cred.Key != ""
}

func writeFile(path string, data []byte) error {
	return ioutil.WriteFile(path, data, fs.ModePerm)
}

func (cred *Credentials) deleteMessages(b *tb.Bot) {
	for _, v := range cred.Pesans {
		_ = b.Delete(v)
	}
}
