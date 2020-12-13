// Copyright 2020 Changkun Ou. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	tg "github.com/go-telegram-bot-api/telegram-bot-api"
)

var (
	token  string
	chatid int64
	bot    *tg.BotAPI
	stores = []string{}
)

func init() {
	token = os.Getenv("TG_BOTTOKEN")
	chid := os.Getenv("TG_CHATID")
	id, err := strconv.Atoi(chid)
	if err != nil {
		panic("chat id is not valid")
	}
	chatid = int64(id)

	if token == "" || chatid == 0 {
		panic("bot token or chat id is empty")
	}

	file, err := os.Open("stores.conf")
	if err != nil {
		panic("cannot open stores.conf")
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		l := scanner.Text()
		if len(l) > 0 && l[0] != '#' {
			stores = append(stores, l)
		}
	}
	log.Println(stores)

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}

	bot, err = tg.NewBotAPI(token)
	if err != nil {
		panic("failed to connect bot")
	}
	bot.Debug = true
	log.Printf("authorized on account %s", bot.Self.UserName)
}

func main() {
	tick := time.NewTicker(time.Minute) // check every minute seems fine for me

	log.Println("start checking...")
	for {
		select {
		case <-tick.C:
			slot, ok := available()
			if !ok {
				log.Println("cannot find appointment")
				continue
			}
			msg := tg.NewMessage(chatid, fmt.Sprintf(msgTmpl, slot.Format(time.RFC822Z)))
			bot.Send(msg)
		}
	}
}

const (
	apAPI   = "https://retail-pz.cdn-apple.com/product-zone-prod/availability/%d-%d-%d/%02d/availability.json"
	msgTmpl = `Appointment avaliable!
time: %v
addr: https://www.apple.com/de/retail/instore-shopping-session/?anchorStore=rosenstrasse
`
)

type errorCode string

const (
	errNotAvailiable errorCode = "NO_TIMESLOT_AVAILABLE"
	errNotNeeded               = "APPOINTMENT_NOT_NEEDED"
)

type entry struct {
	StoreNumber               string    `json:"storeNumber"`
	AppointmentsAvailable     bool      `json:"appointmentsAvailable"`
	FirstAvailableAppointment int64     `json:"firstAvailableAppointment"`
	ErrorCode                 errorCode `json:"errorCode"`
}

func available() (time.Time, bool) {
	now := time.Now().UTC()
	url := fmt.Sprintf(apAPI, now.Year(), now.Month(), now.Day(), now.Hour())
	log.Println("check:", url)

	resp, err := http.Get(url)
	if err != nil {
		log.Println("failed to request the appointment api:", err)
		return time.Time{}, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Println("bad response code:", resp.StatusCode)
		return time.Time{}, false
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println("failed to read response body:", err)
		return time.Time{}, false
	}

	var entries []*entry
	err = json.Unmarshal(b, &entries)
	if err != nil {
		log.Println("failed to parse appointment entries:", err)
		return time.Time{}, false
	}

	for _, e := range entries {
		for _, i := range stores {
			if strings.Compare(e.StoreNumber, i) != 0 {
				continue
			}

			log.Println(e.StoreNumber, e.AppointmentsAvailable, e.FirstAvailableAppointment, e.ErrorCode)
			if e.ErrorCode == errNotNeeded {
				return time.Now(), true
			}
			if e.AppointmentsAvailable {
				return time.Unix(e.FirstAvailableAppointment, 0), true
			}
		}
	}

	return time.Time{}, false
}
