package handler

import (
	"fmt"
	"os"
	"time"

	mybot "lombot/myBot"

	tb "gopkg.in/telebot.v3"
)

func Handle(mb *mybot.MyBot) {
	subscriptions(mb)
	manageUser(mb)
	pse(mb)

	mb.Handle("/Bismillah", func(m tb.Context) error {
		err := mb.Delete(m.Message())
		if err != nil {
			return err
		}

		if m.Message().FromGroup() {
			if mb.IsNewUser(m) {
				return nil
			}

			send, err := mb.Send(m.Chat(), "MasyaaAllah Tabarakallah")
			if err != nil {
				return err
			}
			go mb.DeleteChat(send, 60*time.Second)
			return nil
		}
		if !mb.IsSuperUser(m.Sender().Username) {
			return nil
		}
		send, err := mb.Send(m.Sender(), "MasyaaAllah Tabarakallah")
		if err != nil {
			return err
		}
		go mb.DeleteChat(m.Message(), 60*time.Second)
		go mb.DeleteChat(send, 60*time.Second)
		return nil
	})

	mb.Handle("/admin", func(m tb.Context) error {
		if !m.Message().FromGroup() {
			return nil
		}
		mb.Delete(m.Message())
		if mb.IsNewUser(m) {
			return nil
		}

		mb.Mutex.Lock()
		defer mb.Mutex.Unlock()
		status := mb.HasReportAdmin[m.Chat().ID]
		if status {
			return nil
		}
		mb.HasReportAdmin[m.Chat().ID] = true
		admins, err := mb.AdminsOf(m.Chat())
		if err != nil {
			return err
		}
		res := ""
		for _, admin := range admins {
			if admin.User.Username == m.Sender().Username {
				return nil
			}
			if !admin.User.IsBot && admin.User.Username != "" {
				res += "@" + admin.User.Username + " "
			}
		}
		send, _ := mb.Send(m.Chat(), "Ping <b>"+res+"</b>", tb.ModeHTML)
		t := 5 * time.Minute
		go mb.DeleteChat(send, t)
		go func(t time.Duration, chatID int64) {
			<-time.After(t)
			mb.Mutex.Lock()
			delete(mb.HasReportAdmin, chatID)
			mb.Mutex.Unlock()
		}(t, m.Chat().ID)
		return nil
	})

	mb.Handle("/halo", func(m tb.Context) error {
		if !m.Message().FromGroup() {
			return nil
		}
		mb.Delete(m.Message())
		if mb.IsNewUser(m) {
			return nil
		}
		mb.Send(m.Chat(), fmt.Sprintf("Halo <b>%v!</b> Berembe kabarm?", mybot.GetFullName(m.Sender().FirstName, m.Sender().LastName)), tb.ModeHTML)
		return nil
	})

	mb.Handle("/id", func(m tb.Context) error {
		// all the text messages that weren't
		// captured by existing handlers
		if !m.Message().FromGroup() {
			return nil
		}
		mb.Delete(m.Message())
		if mb.IsNewUser(m) {
			return nil
		}
		msg := fmt.Sprintf("%v, ID Anda adalah %d", mybot.GetFullName(m.Sender().FirstName, m.Sender().LastName), m.Sender().ID)
		mb.Send(m.Chat(), msg)
		return nil
	})

	mb.Handle(tb.OnContact, mb.NotText())
	mb.Handle(tb.OnLocation, mb.NotText())
}

func subscriptions(mb *mybot.MyBot) {
	mb.Handle("/subs", func(m tb.Context) error {
		if !m.Message().FromGroup() {
			return nil
		}
		mb.Delete(m.Message())
		if mb.IsNewUser(m) {
			return nil
		}
		if m.Sender().Username == "" {
			return m.Send("Anda belum memiliki username")
		}

		var count int64
		err := mb.Db.QueryRow("SELECT COUNT(*) as count FROM subscriptions WHERE room_id = ?", m.Chat().ID).
			Scan(&count)
		if err != nil {
			return err
		}

		if count > mb.MaxSubscribers {
			_, err = mb.Send(m.Chat(), "Maaf, batas pendaftaran subscriber telah tercapai")
			return err
		}

		_, err = mb.Db.Exec("insert into subscriptions values (?, ?)", m.Chat().ID, m.Sender().Username)
		if err != nil {
			return err
		}

		send, err := mb.Send(m.Chat(), "Anda sudah subscribe group ini!")
		if err != nil {
			return err
		}
		go mb.DeleteChat(send, 60*time.Second)
		return nil
	})

	mb.Handle("/unsubs", func(m tb.Context) error {
		if !m.Message().FromGroup() {
			return nil
		}
		mb.Delete(m.Message())
		if mb.IsNewUser(m) {
			return nil
		}

		if m.Sender().Username == "" {
			return nil
		}

		_, err := mb.Db.Exec("delete from subscriptions where room_id = ? and user_name = ?", m.Chat().ID, m.Sender().Username)
		if err != nil {
			return err
		}

		send, err := mb.Send(m.Chat(), "Anda sudah tidak subscribe group ini!")
		if err != nil {
			return err
		}
		go mb.DeleteChat(send, 60*time.Second)
		return nil
	})

	mb.Handle("/all", func(m tb.Context) error {
		if !m.Message().FromGroup() {
			return nil
		}

		mb.Delete(m.Message())
		if mb.IsNewUser(m) {
			return nil
		}

		mb.Mutex.Lock()
		defer mb.Mutex.Unlock()

		if mb.HasPublishMessage[m.Chat().ID] {
			return nil
		}

		mb.HasPublishMessage[m.Chat().ID] = true
		var msg string

		rows, err := mb.Db.Query("select user_name from subscriptions where room_id = ?", m.Chat().ID)
		if err != nil {
			return err
		}
		defer rows.Close()

		var batchMessages int64
		var username string
		var max int64
		for rows.Next() {
			err := rows.Scan(&username)
			if err != nil {
				return err
			}

			if username == m.Sender().Username {
				continue
			}

			if max == mb.MaxSubscribers {
				break
			}
			max++

			msg += fmt.Sprintf("@%s ", username)
			if batchMessages == mb.BatchMessagesSubscribers {
				send, err := mb.Send(m.Chat(), msg)
				if err != nil {
					return err
				}
				if mb.SubsDeleteMessageTimeout > 0 {
					go mb.DeleteChat(send, mb.SubsDeleteMessageTimeout*time.Minute)
				}
				msg = ""
			}
		}

		if len(msg) > 0 {
			send, err := mb.Send(m.Chat(), msg)
			if err != nil {
				return err
			}
			if mb.SubsDeleteMessageTimeout > 0 {
				go mb.DeleteChat(send, mb.SubsDeleteMessageTimeout*time.Minute)
			}

		}
		if mb.SubsSpamMessage == 0 {
			delete(mb.HasPublishMessage, m.Chat().ID)
			return nil
		}

		go func() {
			<-time.After(mb.SubsSpamMessage * time.Minute)
			mb.Mutex.Lock()
			delete(mb.HasPublishMessage, m.Chat().ID)
			mb.Mutex.Unlock()
		}()

		return nil
	})
}

func manageUser(mb *mybot.MyBot) {
	mb.Handle(tb.OnUserJoined, func(m tb.Context) error {
		if !m.Message().FromGroup() {
			return nil
		}

		mb.Delete(m.Message())

		if mb.IsSenderAdmin(m.Message()) {
			if !m.Message().UserJoined.IsBot {
				msg := fmt.Sprintf("Selamat datang %v", mybot.GetFullName(m.Message().UserJoined.FirstName, m.Message().UserJoined.LastName))
				mb.Send(m.Chat(), msg)
			}
			return nil
		}

		mb.Mutex.Lock()
		defer mb.Mutex.Unlock()
		_, ok := mb.UserJoin[m.Message().UserJoined.ID]
		if ok {
			return nil
		}

		mb.Retry[m.Message().UserJoined.ID]++
		newMember, err := mb.RestrictUser(m.Message())
		if err != nil {
			send, _ := mb.Send(m.Chat(), "Hai Admin, tolong jadikan saya admin agar dapat mengirim captcha 🙏")
			go mb.DeleteChat(send, 10*time.Minute)
			return nil
		}
		mybot.SaveFileJson(mb.Retry, mb.RetryPath)

		credential := &mybot.Credentials{
			User:   m.Message().UserJoined,
			Pesans: make([]*tb.Message, 0),
			Ch:     make(chan struct{}),
			Wait:   time.Duration(mb.Wait) * time.Minute,
		}

		mb.UserJoin[m.Message().UserJoined.ID] = credential
		imgCaptcha, key, path, err := mybot.GetCaptcha()
		if err != nil {
			return err
		}

		defer func() {
			os.Remove(path)
		}()

		minfo := fmt.Sprintf(`
Hai %v..!
Tulis captcha di bawah dalam waktu %v menit.
<b>Huruf besar dan kecil berpengaruh.
Jika 3 kali salah, maka akan diberi captcha baru.</b>`, mybot.GetFullName(m.Message().UserJoined.FirstName, m.Message().UserJoined.LastName), mb.Wait)

		info, err := mb.Send(m.Chat(), minfo, tb.ModeHTML)
		if err != nil {
			fmt.Println("failed to send msg :", err.Error())
			// Immediately banned user, it's a spam
			mb.Ban(m.Chat(), &tb.ChatMember{User: m.Message().UserJoined, RestrictedUntil: tb.Forever()}, true)
			credential.DeleteMessages(mb.Bot)
			return nil
		}

		captchaMessage, err := mb.Send(m.Chat(), &imgCaptcha)
		if err != nil {
			fmt.Println("failed to send msg :", err.Error())
			// Immediately banned user, it's a spam
			mb.Ban(m.Chat(), &tb.ChatMember{User: m.Message().UserJoined, RestrictedUntil: tb.Forever()}, true)
			credential.DeleteMessages(mb.Bot)
			return nil
		}

		credential.Key = key
		credential.Pesans = append(credential.Pesans, info)
		credential.Pesans = append(credential.Pesans, captchaMessage)

		go mb.AcceptOrDelete(m.Message(), &newMember)
		return nil
	})

	mb.Handle(tb.OnText, func(m tb.Context) error {
		mb.Mutex.Lock()
		defer mb.Mutex.Unlock()
		cred, ok := mb.UserJoin[m.Sender().ID]
		if ok {
			if m.Message().Text == cred.Key {
				mb.Delete(m.Message())
				cred.Ch <- struct{}{}
				return nil
			}
			mb.Delete(m.Message())

			if cred.Retry < 2 {
				cred.Retry++
				return nil
			}

			cred.Retry = 0

			imgCaptcha, key, path, err := mybot.GetCaptcha()
			if err != nil {
				return err
			}
			defer func() {
				os.Remove(path)
			}()

			mb.Edit(cred.Pesans[1], &imgCaptcha)
			cred.Key = key
			return nil
		}
		return nil
	})

	mb.Handle(tb.OnUserLeft, func(m tb.Context) error {
		if !m.Message().FromGroup() {
			return nil
		}
		mb.Delete(m.Message())
		return nil
	})

	mb.Handle("/ban", func(m tb.Context) error {
		mb.Delete(m.Message())
		if !m.Message().FromGroup() {
			return nil
		}
		if !mb.IsSenderAdmin(m.Message()) {
			return nil
		}
		if m.Message().ReplyTo.Sender == nil {
			return nil
		}
		cm, err := mb.ChatMemberOf(m.Chat(), m.Message().ReplyTo.Sender)
		if err != nil {
			return err
		}
		cm.RestrictedUntil = tb.Forever()
		err = mb.Ban(m.Chat(), cm)
		if err != nil {
			return err
		}
		mb.Delete(m.Message().ReplyTo)
		return nil
	})
}

func pse(mb *mybot.MyBot) {
	mb.Handle("/pse", func(m tb.Context) error {
		if !m.Message().FromGroup() {
			return nil
		}

		if mb.IsNewUser(m) {
			return nil
		}

		payload := fmt.Sprintf("%%%s%%", m.Message().Payload)
		rows, err := mb.Db.Query("select name, company, location from pse where name like ?", payload)
		if err != nil {
			return err
		}
		defer rows.Close()

		var msg string
		for rows.Next() {
			var name string
			var company string
			var location string
			err = rows.Scan(&name, &company, &location)
			if err != nil {
				return err
			}
			msg += fmt.Sprintf("NAME : %s\nCOMPANY : %s\nJENIS PERUSAHAAN : %s\n\n", name, company, location)
		}
		if msg == "" {
			msg = "Data tidak ditemukan"
		}
		_, err = mb.Send(m.Chat(), msg, tb.ModeHTML)
		return err
	})
}
