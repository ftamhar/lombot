package mybot

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/steambap/captcha"
	tb "gopkg.in/telebot.v3"
)

type MyBot struct {
	*tb.Bot
	Db                       *sql.DB
	UserJoin                 map[int64]*Credentials
	Retry                    map[int64]int
	HasReportAdmin           map[int64]bool
	HasPublishMessage        map[int64]bool
	Mutex                    *sync.Mutex
	Wait                     int64
	SuperUser                string
	RetryPath                string
	SubsDeleteMessageTimeout time.Duration
	SubsSpamMessage          time.Duration
	MaxSubscribers           int64
	BatchMessagesSubscribers int64
}

type Credentials struct {
	User   *tb.User
	Key    string
	Pesans []*tb.Message
	Wait   time.Duration
	Retry  uint8
	Ch     chan struct{}
}

func (mb *MyBot) IsNewUser(c tb.Context) bool {
	mb.Mutex.Lock()
	_, ok := mb.UserJoin[c.Sender().ID]
	mb.Mutex.Unlock()
	return ok
}

func (myBot *MyBot) DeleteChat(m *tb.Message, t time.Duration) {
	<-time.After(t)
	myBot.Delete(m)
}

func (myBot *MyBot) RestrictUser(m *tb.Message) (tb.ChatMember, error) {
	cm, err := myBot.Bot.ChatMemberOf(m.Chat, m.UserJoined)
	if err != nil {
		fmt.Println("failed to get chat member:", err.Error())
	}

	cm.RestrictedUntil = time.Now().Add(time.Duration(myBot.Retry[m.UserJoined.ID]*5) * time.Minute).Add(time.Duration(myBot.Wait) * time.Minute).Unix()
	cm.CanSendMessages = true
	err = myBot.Bot.Restrict(m.Chat, cm)
	return *cm, nil
}

func (myBot *MyBot) IsSenderAdmin(m *tb.Message) bool {
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

func (mb *MyBot) AcceptOrDelete(m *tb.Message, cm *tb.ChatMember) {
	mb.Mutex.Lock()
	cred := mb.UserJoin[m.UserJoined.ID]
	mb.Mutex.Unlock()
	select {
	case <-time.After(cred.Wait):
		err := mb.Bot.Ban(m.Chat, cm, true)
		if err != nil {
			fmt.Println("failed to ban user:", err.Error())
		}

		cred.DeleteMessages(mb.Bot)
		mb.Mutex.Lock()
		delete(mb.UserJoin, cred.User.ID)
		mb.Mutex.Unlock()
		return

	case <-cred.Ch:
		cm.RestrictedUntil = time.Now().Add(1 * time.Minute).Unix() // if less than 30 seconds, it means forever
		mb.Bot.Restrict(m.Chat, cm)

		cred.DeleteMessages(mb.Bot)
		msg := fmt.Sprintf(`
Selamat datang <b><a href="tg://user?id=%v">%v</a></b>
Anda dapat mengirim media setelah pesan ini hilang`, cred.User.ID, GetFullName(cred.User.FirstName, cred.User.LastName))

		mb.Mutex.Lock()
		delete(mb.UserJoin, cred.User.ID)
		delete(mb.Retry, cred.User.ID)
		mb.Mutex.Unlock()

		SaveFileJson(mb.Retry, mb.RetryPath)
		send, _ := mb.Bot.Send(m.Chat, msg, tb.ModeHTML)
		go mb.DeleteChat(send, time.Minute)
		return
	}
}

func (cred *Credentials) DeleteMessages(b *tb.Bot) {
	for _, v := range cred.Pesans {
		_ = b.Delete(v)
	}
}

func (myBot *MyBot) NotText() tb.HandlerFunc {
	return func(m tb.Context) error {
		if !m.Message().FromGroup() {
			return nil
		}

		_, ok := myBot.UserJoin[m.Sender().ID]
		if ok {
			myBot.Bot.Delete(m.Message())
		}
		return nil
	}
}

func GetFullName(firstName, lastName string) string {
	return strings.Trim(fmt.Sprintf("%v %v", firstName, lastName), " ")
}

func (mb *MyBot) IsSuperUser(username string) bool {
	return username == mb.SuperUser
}

func SaveFileJson(data any, path string) error {
	b1, err := json.Marshal(data)
	if err != nil {
		return err
	}

	if err := writeFile(path, b1); err != nil {
		return err
	}
	return nil
}

func writeFile(path string, data []byte) error {
	return ioutil.WriteFile(path, data, 0o666)
}

func GetCaptcha() (tb.Photo, string, string, error) {
	img, err := captcha.New(300, 100, func(o *captcha.Options) {
		o.Noise = 3
		o.CurveNumber = 13
	})
	if err != nil {
		return tb.Photo{}, "", "", err
	}
	filename := uuid.New()

	pwd, err := os.Getwd()
	if err != nil {
		return tb.Photo{}, "", "", err
	}
	path := pwd + "/" + filename.String() + ".png"
	file, err := os.Create(path)
	if err != nil {
		return tb.Photo{}, "", "", fmt.Errorf("failed to create file : %w", err)
	}
	defer file.Close()

	err = img.WriteImage(file)
	if err != nil {
		return tb.Photo{}, "", "", errors.New("failed to write img")
	}
	return tb.Photo{File: tb.FromDisk(path)}, img.Text, path, nil
}
