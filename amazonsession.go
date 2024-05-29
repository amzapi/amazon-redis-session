package amazonsession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/publicsuffix"

	"github.com/redis/go-redis/v9"
	"github.com/spf13/cast"
)

func sessionIdsKey(country string) string {
	return fmt.Sprintf("%s:session-ids", country)
}

func cookiesKey(country string) string {
	return fmt.Sprintf("%s:cookies", country)
}

func lastCheckedKey(sessionID string) string {
	return fmt.Sprintf("%s:last-checked", sessionID)
}

func usageCountKey(sessionID string) string {
	return fmt.Sprintf("%s:usage-count", sessionID)
}

// defaultCountryCodeDomainMap defines the default Amazon domains for various countries.
var defaultCountryCodeDomainMap = map[string]string{
	"BR": "https://www.amazon.com.br",
	"TR": "https://www.amazon.com.tr",
	"ES": "https://www.amazon.es",
	"FR": "https://www.amazon.fr",
	"IN": "https://www.amazon.in",
	"NL": "https://www.amazon.nl",
	"SA": "https://www.amazon.sa",
	"BE": "https://www.amazon.com.be",
	"AU": "https://www.amazon.com.au",
	"US": "https://www.amazon.com",
	"UK": "https://www.amazon.co.uk",
	"DE": "https://www.amazon.de",
	"SE": "https://www.amazon.se",
	"SG": "https://www.amazon.sg",
	"CA": "https://www.amazon.ca",
	"MX": "https://www.amazon.com.mx",
	"IT": "https://www.amazon.it",
	"AE": "https://www.amazon.ae",
	"PL": "https://www.amazon.pl",
	"JP": "https://www.amazon.co.jp",
}

// AmazonSession is a struct responsible for managing cookies and sessions using Redis.
type AmazonSession struct {
	client redis.UniversalClient
}

// Config holds configuration options for creating a RedisCookieJar instance.
type Config struct {
	// Addr is the address (host:port) of the Redis server.
	Addr string

	// Db is the Redis database number to use.
	Db int

	// Password is the optional password for authenticating with the Redis server.
	Password string
}

type Session struct {
	Jar                 *cookiejar.Jar
	Cookies             []*http.Cookie
	Country             string
	SessionID           string
	UsageCount          int64
	LastCheckedTimeUnix int64
}

func NewAmazonSession(cfg *Config) (*AmazonSession, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.Db,
		DialTimeout:  time.Duration(500) * time.Millisecond,
		WriteTimeout: time.Duration(500) * time.Millisecond,
		ReadTimeout:  time.Duration(5000) * time.Millisecond,
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("failed opening connection to redis: %v", err)
	}
	return &AmazonSession{
		client: rdb,
	}, nil
}

func (j *AmazonSession) GetRandomSession(ctx context.Context, country string) (*Session, error) {
	// Get the total count of session-ids.
	count, err := j.client.LLen(ctx, sessionIdsKey(country)).Result()
	if err != nil {
		return nil, err
	}

	if count == 0 {
		return nil, errors.New("no sessions available for the specified country")
	}

	// Generate a random index.
	randIndex := rand.Int63n(count)

	// Get the session-id at the random index.
	sessionID, err := j.client.LIndex(ctx, sessionIdsKey(country), randIndex).Result()
	if err != nil {
		return nil, err
	}

	return j.GetSession(ctx, country, sessionID)
}

func (j *AmazonSession) PopSession(ctx context.Context, country string) (*Session, error) {
	// Pop a session-id from Redis and remove it from the list.
	key := sessionIdsKey(country)
	sessionID, err := j.client.LPop(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	return j.GetSession(ctx, country, sessionID)
}

func (j *AmazonSession) PushSession(ctx context.Context, session *Session) error {

	if session.Country == "" {
		return fmt.Errorf("country not found in session")
	}

	if session.Jar == nil && (session.Cookies == nil || len(session.Cookies) == 0) {
		return fmt.Errorf("cookies jar and cookies not found in session")
	}

	cookies := session.Cookies

	// Get the cookies from the jar.
	if session.Jar != nil {
		var countryURL *url.URL
		// Check if the country domain exists in the map.
		if domain, found := defaultCountryCodeDomainMap[session.Country]; found {
			// Attempt to parse the domain into a URL.
			countryURL, _ = url.Parse(domain)
		} else {
			return fmt.Errorf("domain not found for country: %s", session.Country)
		}
		cookies = session.Jar.Cookies(countryURL)
	}

	// Store all cookies in a map.
	cookiesMap := make(map[string]string)

	// Check if there is a "session-id" cookie.
	var sessionID string
	for _, item := range cookies {
		if item.Name == "i18n-prefs" ||
			item.Name == "session-id" ||
			item.Name == "session-id-time" ||
			strings.HasPrefix(item.Name, "ubid-") ||
			strings.HasPrefix(item.Name, "lc-") {
			cookiesMap[item.Name] = item.Value
			if item.Name == "session-id" {
				sessionID = item.Value
			}
		}
	}

	// Ensure sessionID is not empty.
	if sessionID == "" {
		return fmt.Errorf("session-id not found in session")
	}

	// Serialize the cookies to JSON.
	cookieData, err := json.Marshal(cookiesMap)
	if err != nil {
		return err
	}

	_, err = j.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {

		// Store the cookies in Redis using Hash data structure.
		key := cookiesKey(session.Country)
		pipe.HSet(ctx, key, sessionID, cookieData)

		lastChecked := time.Now().Unix()
		pipe.HSet(ctx, key, lastCheckedKey(sessionID), lastChecked)
		pipe.HSet(ctx, key, usageCountKey(sessionID), 0)

		// Add the session-id to the list of available session-ids.
		pipe.RPush(ctx, sessionIdsKey(session.Country), sessionID)

		return nil
	})

	if err != nil {
		// Handle the case where the Redis transaction failed
		return fmt.Errorf("redis transaction failed: %v", err)
	}

	return nil
}

func (j *AmazonSession) GetSession(ctx context.Context, country, sessionID string) (*Session, error) {
	countryURL, err := j.getCountryURL(country)
	if err != nil {
		return nil, err
	}

	keys := []string{cookiesKey(country)}
	argv := []interface{}{
		sessionID,
		usageCountKey(sessionID),
		lastCheckedKey(sessionID),
	}

	res, err := getSessionCmd.Run(ctx, j.client, keys, argv...).Result()
	if err != nil {
		return nil, fmt.Errorf("redis eval error: %v", err)
	}

	values, err := cast.ToSliceE(res)
	if err != nil {
		return nil, fmt.Errorf("cast error: Lua script returned unexpected value: %v", res)
	}

	if len(values) != 3 {
		return nil, fmt.Errorf("unepxected number of values returned from Lua script")
	}

	cookieData, err := cast.ToStringE(values[0])
	if err != nil {
		return nil, fmt.Errorf("unexpected value returned from Lua script")
	}

	usageCount, err := cast.ToInt64E(values[1])
	if err != nil {
		return nil, fmt.Errorf("unexpected value returned from Lua script")
	}

	lastCheckedTimeUnix, err := cast.ToInt64E(values[2])
	if err != nil {
		return nil, fmt.Errorf("unexpected value returned from Lua script")
	}

	// Deserialize the JSON data to recreate the cookiejar.Jar.
	cookiesMap := make(map[string]string)
	err = json.Unmarshal([]byte(cookieData), &cookiesMap)
	if err != nil {
		return nil, err
	}

	var cookies []*http.Cookie
	for name, value := range cookiesMap {
		cookies = append(cookies, &http.Cookie{
			Name:    name,
			Value:   value,
			Path:    "/",
			Domain:  countryURL.Host,
			Expires: time.Now().AddDate(1, 0, 0),
		})
	}

	// Create a new cookiejar and set the cookies.
	jar, _ := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	jar.SetCookies(countryURL, cookies)

	return &Session{
		Country:             country,
		Cookies:             cookies,
		Jar:                 jar,
		SessionID:           sessionID,
		UsageCount:          usageCount,
		LastCheckedTimeUnix: lastCheckedTimeUnix,
	}, nil
}

func (j *AmazonSession) GetCountrySessionIDs(ctx context.Context, country string) ([]string, error) {
	return j.client.LRange(ctx, sessionIdsKey(country), 0, -1).Result()
}

func (j *AmazonSession) getCountryURL(country string) (*url.URL, error) {
	var countryURL *url.URL

	// Check if the country domain exists in the map.
	if domain, found := defaultCountryCodeDomainMap[country]; found {
		// Attempt to parse the domain into a URL.
		countryURL, _ = url.Parse(domain)
	} else {
		return nil, fmt.Errorf("domain not found for country: %s", country)
	}

	return countryURL, nil
}

func (j *AmazonSession) GetAllSessions(ctx context.Context) ([]*Session, error) {

	res, err := allSessionCmd.Run(ctx, j.client, nil).Result()
	if err != nil {
		return nil, fmt.Errorf("redis eval error: %v", err)
	}

	data, err := cast.ToSliceE(res)
	if err != nil {
		return nil, fmt.Errorf("cast error: Lua script returned unexpected value: %v", res)
	}

	sessions := make([]*Session, 0)

	for i := 0; i < len(data); i += 5 {

		country := cast.ToString(data[i])
		countryURL, err := j.getCountryURL(country)
		if err != nil {
			return nil, err
		}

		cookieData := cast.ToString(data[i+2])

		// Deserialize the JSON data to recreate the cookiejar.Jar.
		cookiesMap := make(map[string]string)
		err = json.Unmarshal([]byte(cookieData), &cookiesMap)
		if err != nil {
			return nil, err
		}

		var cookies []*http.Cookie
		for name, value := range cookiesMap {
			cookies = append(cookies, &http.Cookie{
				Name:    name,
				Value:   value,
				Path:    "/",
				Domain:  countryURL.Host,
				Expires: time.Now().AddDate(1, 0, 0),
			})
		}

		// Create a new cookiejar and set the cookies.
		jar, _ := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
		jar.SetCookies(countryURL, cookies)

		sessions = append(sessions, &Session{
			Jar:                 jar,
			Cookies:             cookies,
			Country:             cast.ToString(data[i]),
			SessionID:           cast.ToString(data[i+1]),
			UsageCount:          cast.ToInt64(data[i+4]),
			LastCheckedTimeUnix: cast.ToInt64(data[i+3]),
		})
	}

	return sessions, nil
}

func (j *AmazonSession) ListSession(ctx context.Context, country string, pgn Pagination) ([]*Session, error) {
	countryURL, err := j.getCountryURL(country)
	if err != nil {
		return nil, err
	}
	// Note: Because we use LPUSH to redis list, we need to calculate the
	// correct range and reverse the list to get the tasks with pagination.
	stop := -pgn.start() - 1
	start := -pgn.stop() - 1
	res, err := listSessionCmd.Run(ctx, j.client, []string{sessionIdsKey(country), cookiesKey(country)}, start, stop).Result()
	if err != nil {
		return nil, fmt.Errorf("redis eval error: %v", err)
	}
	data, err := cast.ToStringSliceE(res)
	if err != nil {
		return nil, fmt.Errorf("cast error: Lua script returned unexpected value: %v", res)
	}
	allSession := make([]*Session, 0)
	for i := 0; i < len(data); i += 4 {
		cookieData := cast.ToString(data[i+1])
		// Deserialize the JSON data to recreate the cookiejar.Jar.
		cookiesMap := make(map[string]string)
		err = json.Unmarshal([]byte(cookieData), &cookiesMap)
		if err != nil {
			return nil, err
		}
		var cookies []*http.Cookie
		for name, value := range cookiesMap {
			cookies = append(cookies, &http.Cookie{
				Name:    name,
				Value:   value,
				Path:    "/",
				Domain:  countryURL.Host,
				Expires: time.Now().AddDate(1, 0, 0),
			})
		}
		// Create a new cookiejar and set the cookies.
		jar, _ := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
		jar.SetCookies(countryURL, cookies)
		allSession = append(allSession, &Session{
			Jar:                 jar,
			Cookies:             cookies,
			Country:             country,
			SessionID:           cast.ToString(data[i]),
			UsageCount:          cast.ToInt64(data[i+2]),
			LastCheckedTimeUnix: cast.ToInt64(data[i+3]),
		})
	}
	return allSession, nil
}

func (j *AmazonSession) ListCountrySession(ctx context.Context, country string) ([]*Session, error) {
	return j.ListSession(ctx, country, Pagination{
		Size: 0,
		Page: 0,
	})
}

func (j *AmazonSession) UpdateLastCheckedTimestamp(ctx context.Context, country, sessionID string) error {
	// Store the current time as the "last checked" timestamp.
	lastChecked := time.Now().Unix()
	_, err := j.client.HSet(ctx, cookiesKey(country), lastCheckedKey(sessionID), lastChecked).Result()
	if err != nil {
		return err
	}
	return nil
}

func (j *AmazonSession) DeleteSession(ctx context.Context, country, sessionID string) error {
	err := j.client.LRem(ctx, sessionIdsKey(country), 1, sessionID).Err()
	if err != nil {
		return err
	}
	err = j.client.HDel(ctx, cookiesKey(country), sessionID, lastCheckedKey(sessionID), usageCountKey(sessionID)).Err()
	if err != nil {
		return err
	}
	return nil
}

func (j *AmazonSession) CleanupSessions(ctx context.Context, timeDiffThreshold int64, usageCountThreshold int64) error {
	args := []interface{}{
		time.Now().Unix(),
		timeDiffThreshold,
		usageCountThreshold,
	}
	if err := cleanupSessionsCmd.Run(ctx, j.client, []string{}, args...).Err(); err != nil {
		return fmt.Errorf("redis eval error: %v", err)
	}
	return nil
}

func (j *AmazonSession) ClearAllCookies(ctx context.Context) error {
	for country := range defaultCountryCodeDomainMap {
		err := j.client.Del(ctx, sessionIdsKey(country)).Err()
		if err != nil {
			return fmt.Errorf("failed to delete session IDs for country %s: %v", country, err)
		}
		err = j.client.Del(ctx, cookiesKey(country)).Err()
		if err != nil {
			return fmt.Errorf("failed to delete cookies for country %s: %v", country, err)
		}
	}
	return nil
}
