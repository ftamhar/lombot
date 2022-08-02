package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"lombot/database"
	mybot "lombot/myBot"

	"lombot/handler"

	"github.com/rs/zerolog"

	tb "gopkg.in/telebot.v3"
)

var (
	pwd                      string
	retryPath                string
	token                    string
	superUser                string
	subsDeleteMessageTimeout int
	wait                     int64
	ignore                   int64
	verbose                  bool
	maxSubscribers           int64
	batchMessagesSubscribers int64
	subsSpamMessage          int64
)

func init() {
	var err error
	pwd, err = os.Getwd()
	if err != nil {
		log.Panicf("failed to get current directory: %v", err.Error())
	}
	retryPath = pwd + "/retry.json"
	flag.StringVar(&token, "t", "", "bot token")
	flag.StringVar(&superUser, "u", "", "super user")
	flag.Int64Var(&wait, "w", 5, "lama menunggu jawaban (menit)")
	flag.Int64Var(&ignore, "i", 10, "lama mengabaikan chat (detik)")
	flag.IntVar(&subsDeleteMessageTimeout, "sdmt", 0, "timeout subscription (menit)")
	flag.Int64Var(&maxSubscribers, "ms", 100, "max subscriber")
	flag.Int64Var(&batchMessagesSubscribers, "bms", 30, "batch subscribe")
	flag.Int64Var(&subsSpamMessage, "ssm", 0, "subs spam message")

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
	if token == "" {
		log.Fatal("Token harus diisi : -t <token>")
	}
	if superUser == "" {
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
		if time.Since(u.Message.Time()) > time.Duration(ignore)*time.Second {
			return false
		}
		return true
	})
	fileLogger, err := os.OpenFile("./logger.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer fileLogger.Close()

	myLogger := zerolog.New(fileLogger).With().Timestamp().Caller().Logger()

	b, err := tb.NewBot(tb.Settings{
		// You can also set custom API URL.
		// If field is empty it equals to "https://api.telegram.org".
		// URL: "http://195.129.111.17:8012",

		Token: token,
		// Poller: &tb.LongPoller{Timeout: 10 * time.Second},
		Poller: middleware,

		// sync true to ensure all messages deleted properly
		Synchronous: true,
		Verbose:     verbose,
	})
	if err != nil {
		log.Fatal("token salah: " + err.Error())
		return
	}
	defer b.Stop()

	b.OnError = func(err error, c tb.Context) {
		if err != tb.ErrTrueResult {
			myLogger.Error().Err(err).Msg(c.Message().Text)
		}
	}

	defer func() {
		if r := recover(); r != nil {
			msg := fmt.Sprintf("Type : %T; Value : %v", r, r)
			myLogger.Error().Msg(msg)
		}
	}()

	db, err := database.OpenSqlite()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	myBot := &mybot.MyBot{
		Bot:                      b,
		Db:                       db,
		UserJoin:                 make(map[string]*mybot.Credentials),
		Retry:                    make(map[string]int),
		HasReportAdmin:           make(map[int64]bool),
		HasPublishMessage:        make(map[int64]bool),
		Mutex:                    &sync.Mutex{},
		Wait:                     wait,
		SuperUser:                superUser,
		RetryPath:                retryPath,
		SubsDeleteMessageTimeout: time.Duration(subsDeleteMessageTimeout),
		SubsSpamMessage:          time.Duration(subsSpamMessage),
		MaxSubscribers:           maxSubscribers,
		BatchMessagesSubscribers: batchMessagesSubscribers,
	}

	fileRetry, err := os.ReadFile(retryPath)
	if err != nil {
		log.Fatal("failed to read file retry:", err.Error())
		return
	}

	if len(fileRetry) > 0 {
		err := json.Unmarshal(fileRetry, &myBot.Retry)
		if err != nil {
			log.Fatal("failed to unmarshal retry:", err.Error())
		}
	}

	ch := make(chan struct{})
	go func() {
		generateAt := checkPseTime()
		urls := []database.PSETerdaftar{
			{
				Url:      "https://pse.kominfo.go.id/static/json-static/ASING_TERDAFTAR/",
				Location: "Asing",
			},
			{
				Url:      "https://pse.kominfo.go.id/static/json-static/LOKAL_TERDAFTAR/",
				Location: "Domestik",
			},
		}
		err := database.UpdatePseSqlite(myBot, urls)
		if err != nil {
			log.Fatal(err)
		}
		ch <- struct{}{}
		for {
			time.Sleep(1 * time.Hour)
			log.Println("checking data pse")
			generateAtNow := checkPseTime()
			if generateAt == generateAtNow || generateAtNow == "" {
				log.Println("no change")
				continue
			}
			generateAt = generateAtNow
			database.UpdatePseSqlite(myBot, urls)
			log.Println("data pse updated")
		}
	}()
	<-ch
	close(ch)
	log.Println("finish loading data pse")
	handler.Handle(myBot)
	fmt.Println("bot started")
	b.Start()
}

func writeFile(path string, data []byte) error {
	return ioutil.WriteFile(path, data, 0o666)
}

func checkPseTime() string {
	type responseCheck struct {
		Data struct {
			GeneratedAt string `json:"generated_at"`
		} `json:"data"`
	}
	client := http.DefaultClient
	res, err := client.Get("https://pse.kominfo.go.id/static/json-static/generationInfo.json")
	if err != nil {
		log.Println("failed to get generation info:", err.Error())
		return ""
	}

	var response responseCheck
	err = json.NewDecoder(res.Body).Decode(&response)
	res.Body.Close()
	if err != nil {
		log.Println("failed to decode generation info:", err.Error())
		return ""
	}
	return response.Data.GeneratedAt
}
