package main

import (
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/ansel1/merry"
	"github.com/go-resty/resty/v2"
	jsoniter "github.com/json-iterator/go"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type myNews struct {
	TagNews   []*News `json:"tagNews"`
	Timestamp int64   `json:"timestamp"`
	ListEnd   bool    `json:"listEnd"`
}

type News struct {
	ID    int64  `json:"id"`
	Mark  string `json:"mark"`
	Title string `json:"title"`
	Link  string `json:"link"`
	Date  int64  `json:"date"`
}

type req struct {
	TagSlug       string `json:"tagSlug"`
	Limit         int64  `json:"limit"`
	LastTimestamp *int64 `json:"lastTimestamp"`
}

func main() {
	log.Logger = log.
		Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.StampMicro}).
		With().Caller().Timestamp().Logger()
	zerolog.TimeFieldFormat = time.StampMicro

	newsCH := make(chan *News, 1000)

	wg := sync.WaitGroup{}
	wg.Add(1)

	go loadNews(newsCH, &wg)
	go writeNews(newsCH, "./result.txt", &wg)

	time.Sleep(time.Second)
	wg.Done()
	wg.Wait()

	log.Debug().Msg(fmt.Sprintf("end"))
}

func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}

// jConv convert go types into string for JSON encoding
// nolint:gocyclo
func jConv(v interface{}) string {
	switch x := v.(type) {
	case int:
		return fmt.Sprintf("%d", x)
	case int64:
		return fmt.Sprintf("%d", x)
	case bool:
		if x {
			return "1"
		}

		return "0"
	case []byte:
		return string(x)
	case string:
		return x
	case []int64:
		j, err := jsoniter.Marshal(x)
		if err != nil {
			log.Error().Err(err).Str("errors_stack", merry.Details(err)).
				Interface("value", x).Msg("err marshaling []int64 to JSON")
		}

		return string(j)
	case []string:
		var ret string

		if len(x) == 0 {
			return ret
		}

		ret = x[0]
		for i := 1; i < len(x); i++ {
			ret += "," + x[i]
		}

		return ret
	default:
		log.Error().Interface("value", x).Msg("jConv unsupported type")
		return ""
	}
}

func loadNews(newsCH chan *News, wg *sync.WaitGroup) {
	wg.Add(1)
	defer close(newsCH)

	client := resty.New()
	url := "https://tass.ru/userApi/tagNews"
	params := req{
		TagSlug:       "krizis-na-ukraine",
		Limit:         10,
		LastTimestamp: nil,
	}

	for {
		resp, err := client.R().
			SetBody(params).
			SetHeader("Content-Type", "application/json").
			Post(url)
		if err != nil {
			log.Debug().Msg(fmt.Sprintf("resty request err: '%v'", err))
			return
		}

		if resp.StatusCode() != http.StatusOK {
			log.Debug().Msg(fmt.Sprintf("resty not OK status code: '%d'", resp.StatusCode()))
			return
		}

		log.Debug().Msg(fmt.Sprintf("response: '%s'", resp.Body()))

		news := myNews{}
		if err := jsoniter.Unmarshal(resp.Body(), &news); err != nil {
			log.Debug().Msg(fmt.Sprintf("jsoniter unmarshall error: '%v'", err))
			return
		}

		oldestTimestamp := int64(0)

		for _, n := range news.TagNews {
			if oldestTimestamp == 0 {
				oldestTimestamp = n.Date
			}

			if oldestTimestamp > n.Date {
				oldestTimestamp = n.Date
			}

			newsCH <- n
		}

		params.LastTimestamp = &oldestTimestamp

		if news.ListEnd {
			return
		}

		time.Sleep(time.Millisecond * 100)
	}
}

func writeNews(newsCH chan *News, path string, wg *sync.WaitGroup) {
	defer wg.Done()

	f, err := os.Create(path)
	if err != nil {
		log.Debug().Msg(fmt.Sprintf("open file err: '%v", err))
		return
	}
	defer func() {
		err := f.Close()
		if err != nil {
			log.Debug().Msg(fmt.Sprintf("closing file err: '%v'", err))
		}
	}()

	for n := range newsCH {
		tm := time.Unix(n.Date, 0)
		fmt.Println(tm)

		s := fmt.Sprintf("%s %s <%s>\n", tm.Format("2006-01-02T03:04:05"), n.Title, n.Link)

		l, err := f.WriteString(s)
		if err != nil {
			log.Debug().Msg(fmt.Sprintf("write file err: '%v'", err))
			return
		}

		fmt.Println(l, "bytes written successfully")
	}
}
