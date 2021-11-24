package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/steambap/captcha"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	tb "gopkg.in/tucnak/telebot.v2"
)

var (
	pwd       string
	retryPath string
	token     *string
	superUser *string
	wait      *int64
	ignore    *int64
	verbose   bool
	mutex     sync.Mutex
)

type Credentials struct {
	User   *tb.User
	Key    string
	Pesans []*tb.Message
	ch     chan struct{}
	wait   time.Duration
}

type MyBot struct {
	Bot            *tb.Bot
	UserJoin       map[int]*Credentials
	retry          map[int]int
	hasReportAdmin bool
}

func init() {
	pwd, _ = os.Getwd()
	retryPath = pwd + "/retry.json"
	token = flag.String("t", "", "token bot telegram (required)")
	superUser = flag.String("u", "", "username pengelola bot (required)")
	wait = flag.Int64("w", 5, "lama menunggu jawaban (menit)")
	ignore = flag.Int64("i", 60, "lama mengabaikan chat (detik)")

	flag.Func("v", "mode debug (boolean) (default false)", func(s string) error {
		if s != "true" && s != "false" {
			return errors.New("wrong format, must be \"true\" or \"false\"")
		}
		if s == "true" {
			verbose = true
			return nil
		}
		return nil
	})
}

func main() {
	flag.Parse()
	if *token == "" {
		log.Fatal("Token harus diisi : -t <token>")
	}
	if *superUser == "" {
		log.Fatal("username pengelola bot harus diisi : -u <username>")
	}

	if _, err := os.Stat(retryPath); errors.Is(err, os.ErrNotExist) {
		err := writeFile(retryPath, []byte(""))
		if err != nil {
			log.Panicf("error to write file : %v", err.Error())
		}
	}

	poller := &tb.LongPoller{Timeout: 10 * time.Second}

	middleware := tb.NewMiddlewarePoller(poller, func(u *tb.Update) bool {
		if u.Message == nil {
			return true
		}
		// ignore chat in (*ignore) time
		if time.Since(u.Message.Time()) > time.Duration(*ignore)*time.Second {
			return false
		}
		return true
	})
	fileLogger, err := os.OpenFile("./logger.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer fileLogger.Close()

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	myLogger := zerolog.New(fileLogger).With().Timestamp().Caller().Logger()

	b, err := tb.NewBot(tb.Settings{
		// You can also set custom API URL.
		// If field is empty it equals to "https://api.telegram.org".
		// URL: "http://195.129.111.17:8012",

		Token: *token,
		// Poller: &tb.LongPoller{Timeout: 10 * time.Second},
		Poller: middleware,

		// sync true to ensure all messages deleted properly
		Synchronous: true,
		Reporter: func(err error) {
			myLogger.Error().Msg(err.Error())
		},
		Verbose: verbose,
	})

	if err != nil {
		log.Fatal("token salah: " + err.Error())
		return
	}

	defer b.Stop()

	defer func() {
		if r := recover(); r != nil {
			msg := fmt.Sprintf("Type : %T; Value : %v", r, r)
			myLogger.Error().Msg(msg)
		}
	}()

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

	b.Handle("/Bismillah", func(m *tb.Message) {
		if m.FromGroup() {
			send, _ := b.Send(m.Chat, "MasyaaAllah Tabarakallah")
			go myBot.deleteChat(m, 60*time.Second)
			go myBot.deleteChat(send, 60*time.Second)
			return
		}
		if !isSuperUser(m.Sender.Username) {
			return
		}
		send, _ := b.Send(m.Sender, "MasyaaAllah Tabarakallah")
		go myBot.deleteChat(m, 60*time.Second)
		go myBot.deleteChat(send, 60*time.Second)
	})

	b.Handle("/admin", func(m *tb.Message) {
		if !m.FromGroup() {
			return
		}
		b.Delete(m)
		mutex.Lock()
		status := myBot.hasReportAdmin
		mutex.Unlock()
		if status {
			return
		}
		mutex.Lock()
		myBot.hasReportAdmin = true
		mutex.Unlock()
		admins, err := b.AdminsOf(m.Chat)
		if err != nil {
			panic("failed to get admin")
		}
		res := ""
		for _, admin := range admins {
			if admin.User.Username == m.Sender.Username {
				return
			}
			if !admin.User.IsBot && admin.User.Username != "" {
				res += "@" + admin.User.Username + " "
			}
		}
		send, _ := b.Send(m.Chat, "Ping "+res)
		t := 30 * time.Minute
		go myBot.deleteChat(send, t)
		go func(t time.Duration) {
			select {
			case <-time.After(t):
				mutex.Lock()
				myBot.hasReportAdmin = false
				mutex.Unlock()
			}
		}(t)
	})

	b.Handle("/ban", func(m *tb.Message) {
		b.Delete(m)
		if !m.FromGroup() {
			return
		}
		if !myBot.isSenderAdmin(m) {
			return
		}
		cm, err := b.ChatMemberOf(m.Chat, m.ReplyTo.Sender)
		if err != nil {
			panic("failed to get chat member: " + err.Error())
		}
		cm.RestrictedUntil = tb.Forever()
		err = b.Ban(m.Chat, cm)
		if err != nil {
			panic(err.Error())
		}
		b.Delete(m.ReplyTo)
	})

	b.Handle("/halo", func(m *tb.Message) {
		if !m.FromGroup() {
			return
		}
		b.Send(m.Chat, fmt.Sprintf("Halo %v! Berembe kabarm?", getFullName(m.Sender.FirstName, m.Sender.LastName)))
	})

	b.Handle("/id", func(m *tb.Message) {
		// all the text messages that weren't
		// captured by existing handlers
		if !m.FromGroup() {
			return
		}
		msg := fmt.Sprintf("%v, ID Anda adalah %d", getFullName(m.Sender.FirstName, m.Sender.LastName), m.Sender.ID)
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

		b.Delete(m)

		if myBot.isSenderAdmin(m) {
			if !m.UserJoined.IsBot {
				msg := fmt.Sprintf("Selamat datang %v", getFullName(m.UserJoined.FirstName, m.UserJoined.LastName))
				b.Send(m.Chat, msg)
			}
			return
		}

		mutex.Lock()
		credential, ok := myBot.UserJoin[m.UserJoined.ID]
		mutex.Unlock()
		if ok {
			return
		}

		myBot.retry[m.UserJoined.ID]++
		cm, err := myBot.restrictUser(m)
		if err != nil {
			send, _ := b.Send(m.Chat, "Hai Admin, tolong jadikan saya admin agar dapat mengirim captcha 🙏")
			go myBot.deleteChat(send, 30*time.Minute)
			return
		}
		saveFileJson(myBot.retry, retryPath)

		credential = &Credentials{
			User:   m.UserJoined,
			Pesans: make([]*tb.Message, 0),
			ch:     make(chan struct{}),
			wait:   time.Duration(*wait) * time.Minute,
		}

		mutex.Lock()
		myBot.UserJoin[m.UserJoined.ID] = credential
		mutex.Unlock()

		imgCaptcha, key, path, err := getCaptcha()
		if err != nil {
			panic(err.Error())
		}

		defer func() {
			os.Remove(path)
		}()

		minfo := fmt.Sprintf(`
Hai %v..!
Tulis captcha di bawah dalam waktu %v menit.
Huruf besar dan kecil berpengaruh`, getFullName(m.UserJoined.FirstName, m.UserJoined.LastName), *wait)

		info, err := b.Send(m.Chat, minfo)
		if err != nil {
			fmt.Println("failed to send msg :", err.Error())
			// Immediately banned user, it's a spam
			b.Ban(m.Chat, &tb.ChatMember{User: m.UserJoined, RestrictedUntil: tb.Forever()}, true)
			credential.deleteMessages(b)
			return
		}

		captchaMessage, err := b.Send(m.Chat, &imgCaptcha)
		if err != nil {
			fmt.Println("failed to send msg :", err.Error())
			// Immediately banned user, it's a spam
			b.Ban(m.Chat, &tb.ChatMember{User: m.UserJoined, RestrictedUntil: tb.Forever()}, true)
			credential.deleteMessages(b)
			return
		}

		credential.Key = key
		credential.Pesans = append(credential.Pesans, info)
		credential.Pesans = append(credential.Pesans, captchaMessage)

		go myBot.acceptOrDelete(m, &cm)
	})

	b.Handle(tb.OnText, func(m *tb.Message) {
		mutex.Lock()
		cred, ok := myBot.UserJoin[m.Sender.ID]
		mutex.Unlock()
		if ok {
			if m.Text == cred.Key {
				b.Delete(m)
				cred.ch <- struct{}{}
				return
			}
			b.Delete(m)

			imgCaptcha, key, path, err := getCaptcha()
			if err != nil {
				panic(err.Error())
			}
			defer func() {
				os.Remove(path)
			}()

			b.Edit(cred.Pesans[1], &imgCaptcha)
			mutex.Lock()
			myBot.UserJoin[m.Sender.ID].Key = key
			mutex.Unlock()
		}
	})

	b.Handle(tb.OnUserLeft, func(m *tb.Message) {
		if !m.FromGroup() {
			return
		}
		b.Delete(m)
	})

	b.Handle(tb.OnContact, myBot.notText())
	b.Handle(tb.OnLocation, myBot.notText())

	fmt.Println("bot started")
	b.Start()
}

func getCaptcha() (tb.Photo, string, string, error) {
	img, err := captcha.New(300, 100, func(o *captcha.Options) {
		o.Noise = 3
		o.CurveNumber = 13
	})
	filename := uuid.New()
	path := pwd + "/" + filename.String() + ".png"
	file, err := os.Create(path)
	if err != nil {
		return tb.Photo{}, "", "", errors.New("failed to open c.png")
	}
	defer func() {
		file.Close()
	}()

	err = img.WriteImage(file)
	if err != nil {
		return tb.Photo{}, "", "", errors.New("failed to write img")
	}
	return tb.Photo{
		File: tb.FromDisk(path),
	}, img.Text, path, nil
}

func getFullName(f, l string) string {
	return strings.Trim(fmt.Sprintf("%v %v", f, l), " ")
}

func isSuperUser(username string) bool {
	return username == *superUser
}

func (myBot *MyBot) deleteChat(m *tb.Message, t time.Duration) {
	select {
	case <-time.After(t):
		myBot.Bot.Delete(m)
	}
}

func (myBot *MyBot) restrictUser(m *tb.Message) (tb.ChatMember, error) {
	cm, err := myBot.Bot.ChatMemberOf(m.Chat, m.UserJoined)
	if err != nil {
		fmt.Println("failed to get chat member:", err.Error())
	}

	cm.RestrictedUntil = time.Now().Add(time.Duration(myBot.retry[m.UserJoined.ID]*5) * time.Minute).Add(time.Duration(*wait) * time.Minute).Unix()
	cm.CanSendMessages = true
	err = myBot.Bot.Restrict(m.Chat, cm)
	if err != nil {
		return *cm, err
	}
	return *cm, nil
}

func (myBot *MyBot) isSenderAdmin(m *tb.Message) bool {
	admins, err := myBot.Bot.AdminsOf(m.Chat)
	if err != nil {
		return false
	}

	for _, v := range admins {
		if v.User.ID == m.Sender.ID {
			return true
		}
	}
	return false
}

func (myBot *MyBot) acceptOrDelete(m *tb.Message, cm *tb.ChatMember) {
	mutex.Lock()
	cred := myBot.UserJoin[m.UserJoined.ID]
	mutex.Unlock()
	select {
	case <-time.After(cred.wait):
		err := myBot.Bot.Ban(m.Chat, cm, true)
		if err != nil {
			fmt.Println("failed to ban user:", err.Error())
		}

		cred.deleteMessages(myBot.Bot)
		mutex.Lock()
		delete(myBot.UserJoin, m.UserJoined.ID)
		mutex.Unlock()
		return

	case <-cred.ch:
		cred.deleteMessages(myBot.Bot)
		mutex.Lock()
		delete(myBot.UserJoin, m.UserJoined.ID)
		mutex.Unlock()

		msg := fmt.Sprintf("Selamat datang %v %v", cred.User.FirstName, cred.User.LastName)

		mutex.Lock()
		delete(myBot.retry, m.UserJoined.ID)
		mutex.Unlock()
		saveFileJson(myBot.retry, retryPath)

		myBot.Bot.Send(m.Chat, msg)
		return
	}
}

func saveFileJson(data interface{}, path string) {
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

		_, ok := myBot.UserJoin[m.Sender.ID]
		if ok {
			myBot.Bot.Delete(m)
		}
	}
}

func writeFile(path string, data []byte) error {
	return ioutil.WriteFile(path, data, 0666)
}

func (cred *Credentials) deleteMessages(b *tb.Bot) {
	for _, v := range cred.Pesans {
		_ = b.Delete(v)
	}
}
