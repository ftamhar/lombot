package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
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
	pwd       string
	retryPath string
	token     string
	superUser string
	wait      int64
	ignore    int64
	verbose   bool
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

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
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

	defer func() {
		if r := recover(); r != nil {
			msg := fmt.Sprintf("Type : %T; Value : %v", r, r)
			myLogger.Error().Msg(msg)
		}
	}()

	db, err := database.OpenDbConnection()
	if err != nil {
		log.Fatal(err)
	}

	myBot := &mybot.MyBot{
		Bot:       b,
		Db:        db,
		UserJoin:  make(map[int64]*mybot.Credentials),
		Retry:     make(map[int64]int),
		Mutex:     sync.Mutex{},
		Wait:      wait,
		SuperUser: superUser,
		RetryPath: retryPath,
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

	handler.Handle(myBot)
	fmt.Println("bot started")
	b.Start()
}

func writeFile(path string, data []byte) error {
	return ioutil.WriteFile(path, data, 0o666)
}
