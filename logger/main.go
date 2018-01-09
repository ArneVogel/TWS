package main

import (
	"bufio"
	"database/sql"
	"fmt"
	_ "github.com/lib/pq"
	"net"
	"strings"
	"time"
	"flag"
	"github.com/joshbetz/config"
)

var defunct []string

// can be set with -config and -debug
var configFile string
var debug bool

// Loaded from config.json
var DB_USER string
var DB_NAME string
var DB_PASSWORD string
var joinLimit time.Duration
var joinLimitInt int

func dbWriter(query string, args ...string) {
	dbinfo := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable", DB_USER, DB_PASSWORD, DB_NAME)
	db, err := sql.Open("postgres", dbinfo)
	if err != nil {
		fmt.Println("Couln't open db in dbWriter: ", err)
		return
	}
	defer db.Close()

	// Creating an interface from the args slice.
	// https://golang.org/doc/faq#convert_slice_of_interface
	argsi := make([]interface{}, len(args))
	for i, v := range args {
		argsi[i] = v
	}

	// dummy is needed so the connection is closed after executing the query
	// without .Scan(&dummy) t
	var dummy string
	err = db.QueryRow(query, argsi...).Scan(&dummy)
	if err != nil {
		fmt.Println("Couln't execute query db in dbWriter: ", err)
		return
	}
}

func subscriptionHander(msg string) {
	sep := strings.Split(msg, ";")
	var display_name, name, user_id, partner_id, month, plan, sub_msg, streamer, id string
	for _, s := range sep {
		if strings.Contains(s, "display-name=") && !strings.Contains(s, "msg-param") {
			display_name = strings.TrimPrefix(s, "display-name=")
		}
		if strings.Contains(s, "login=") {
			name = strings.TrimPrefix(s, "login=")
		}
		if strings.Contains(s, "user-id=") {
			user_id = strings.TrimPrefix(s, "user-id=")
		}
		if strings.Contains(s, "msg-param-sub-plan=") {
			plan = strings.TrimPrefix(s, "msg-param-sub-plan=")
		}
		if strings.Contains(s, "room-id=") {
			partner_id = strings.TrimPrefix(s, "room-id=")
		}
		if strings.Contains(s, "msg-param-months=") {
			month = strings.TrimPrefix(s, "msg-param-months=")
		}
		if strings.Contains(s, ":tmi.twitch.tv USERNOTICE #") {
			tmp := strings.Split(s, ":")
			streamer = strings.TrimPrefix(tmp[1], "tmi.twitch.tv USERNOTICE #")
			if len(tmp) > 2 { // if an sub message was sent
				sub_msg = tmp[2]
			}
		}
		if strings.Split(s, "=")[0] == "id" {
			id = strings.TrimPrefix(s, "id=")
		}
	}
	curr_time := time.Now()
	date := fmt.Sprintf(curr_time.Format("2006-01-02"))

	dbWriter(`INSERT INTO users(id, display_name, name) VALUES($1, $2, $3) ON CONFLICT (id) DO UPDATE SET display_name = $2, name = $3 returning id`, user_id, display_name, name)

	dbWriter(`INSERT INTO subscription(u_id, p_id, month, sub_plan, msg, date, id) VALUES($1, $2, $3, $4, $5, $6, $7) returning u_id`, user_id, partner_id, month, plan, sub_msg, date, id)

	fmt.Printf("%v subscribed to %v for %v months on %v\n", name, streamer, month, date)
}


func listener(stream string) {
	conn, err := net.Dial("tcp", "irc.chat.twitch.tv:6667")
	if conn != nil {
		defer conn.Close()
	}
	
	if err != nil {
		fmt.Printf("error during tcp connection for stream: %v, err: %v\n", stream, err)
		defunct = append(defunct, stream)
		return
	}
	conn.Write([]byte("PASS " + "oauth:rdx4jdrurv49ot8s01ph3jeu20ux90" + "\r\n"))
	conn.Write([]byte("NICK " + "anon0323" + "\r\n"))
	conn.Write([]byte("JOIN " + "#" + stream + "\r\n"))
	conn.Write([]byte("CAP REQ :twitch.tv/commands\r\n"))
	conn.Write([]byte("CAP REQ :twitch.tv/tags\r\n"))
	
	if debug {
		fmt.Println("joined:", stream)
	}
	
	reader := bufio.NewReader(conn)
	
	for {
		msgByte, _, err := reader.ReadLine()
		msg := string(msgByte)
		if err != nil {
			if debug {
				fmt.Printf("Error during readline: %v in stream: %v\n", err, stream)
			}
			defunct = append(defunct, stream)
			return
		}
		msgParts := strings.Split(msg, " ")

		if msgParts[0] == "PING" {
			conn.Write([]byte("PONG " + msgParts[1]))
			continue
		}
		if strings.Contains(msg, ":tmi.twitch.tv USERNOTICE") && strings.Contains(msg, "msg-param-months")  {
			go subscriptionHander(msg)
			if debug {
				fmt.Println(msg)
			}
			
		}
	}

}

func dbReader(query string) *sql.Rows {
	dbinfo := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable", DB_USER, DB_PASSWORD, DB_NAME)
	db, err := sql.Open("postgres", dbinfo)
	defer db.Close()
	if err != nil {
		fmt.Println("Couln't open db in dbReader: ", err)
		return nil
	}
	rows, err := db.Query(query)
	if err != nil {
		fmt.Println("Error during query: ", err)
		return nil
	}
	return rows
}

func getPartnersNames(rows *sql.Rows) []string {
	var partners []string
	for rows.Next() {
		var name string
		err := rows.Scan(&name)
		if err != nil {
			fmt.Println("error in getPartnersNames: ", err)
			return nil
		}
		partners = append(partners, name)
	}
	return partners
}

func master() {
	rate := time.Minute / joinLimit
	throttle := time.Tick(rate)

	var toJoin map[string]bool
	toJoin = make(map[string]bool) // false = already joined, true = join on next opportunity

	for {
		toMap := getPartnersNames(dbReader("SELECT name FROM partner"))
		for _, e := range toMap {
			if !toJoin[e] {
				toJoin[e] = false
			}
		}

		// TODO possible problem: some stream could get defunct during this loop => defunct gets set to nil => stream wont be rejoined
		for _, e := range defunct {
			toJoin[e] = false
		}
		defunct = nil
		for stream, join := range toJoin {
			if !join {
				<-throttle
				go listener(stream)
				toJoin[stream] = true
			}
		}
		//fmt.Println("finished")
		time.Sleep(10 * time.Second)
	}
}

func init() {
	configFlag := flag.String("config", "../config.json", "use -config if the config file is not named config.json or is in a different folder than the tws binary")
	debugFlag := flag.Bool("debug", false, "enable debug output")
	flag.Parse()
	
	debug = *debugFlag
	configFile = *configFlag
	
	c := config.New(configFile)
	err := c.Get("DN_NAME", &DB_NAME)
	if err != nil && debug {
		fmt.Println("Couldn't get DB_NAME from config file: ", configFile)
	}
	err = c.Get("DB_USER", &DB_USER)
	if err != nil && debug {
		fmt.Println("Couldn't get DB_USER from config file: ", configFile)
	}
	err = c.Get("DB_PASSWORD", &DB_PASSWORD)
	if err != nil && debug {
		fmt.Println("Couldn't get DB_PASSWORD from config file: ", configFile)
	}
	err = c.Get("JOIN_PER_MINUTE_LIMIT", &joinLimitInt)
	joinLimit = time.Duration(joinLimitInt)
	if err != nil && debug {
		fmt.Println("Couldn't get JOIN_LIMIT from config file: ", configFile)
	}
	if debug {
		fmt.Printf("DB_NAME: %v, DB_USER: %v, DB_PASSWORD: %v, joinLimit: %v\n", DB_NAME, DB_USER, DB_PASSWORD, joinLimit)
	}
}

func main() {
	master()
}
