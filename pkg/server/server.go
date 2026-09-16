package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/text/language"

	"github.com/pinheirolucas/peace-breaker-bot/pkg/bot"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/fsutil"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/httpclient"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/i18n"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/instant"
)

const autodiscoveryServiceName = "_myinstants._tcp"

// defaultClient scrapes myinstants.com when Server.client is unset. It has to
// be the httpclient one: myinstants.com answers 403 to Go's default User-Agent.
var defaultClient = httpclient.New()

// BotStatus is the bot's voice-connection state, as the server needs it.
type BotStatus interface {
	Status() bot.VoiceStatus
}

type Server struct {
	player *instant.Player
	bot    BotStatus

	// myInstantsBaseURL and client let tests point the scrape at a fixture
	// server; both fall back to the production values when unset.
	myInstantsBaseURL string
	client            *http.Client
}

func New(player *instant.Player, bot BotStatus) *Server {
	return &Server{player: player, bot: bot}
}

func (s *Server) baseURL() string {
	if s.myInstantsBaseURL != "" {
		return s.myInstantsBaseURL
	}

	return "https://www.myinstants.com"
}

func (s *Server) httpClient() *http.Client {
	if s.client != nil {
		return s.client
	}

	return defaultClient
}

func (s *Server) Start(address string) error {
	r := http.NewServeMux()

	r.HandleFunc("POST /api/v1/bot/play", s.handleBotPlay)
	r.HandleFunc("POST /api/v1/bot/stop", s.handleBotStop)
	r.HandleFunc("GET /api/v1/bot/status", s.handleBotStatus)
	r.HandleFunc("GET /api/v1/instants", s.handleListInstants)
	r.HandleFunc("GET /api/v1/instants/{url}/content", s.handleInstantContent)
	r.HandleFunc("GET /api/v1/openapi.yaml", s.handleOpenAPISpec)
	r.HandleFunc("GET /api/docs", s.handleDocs)

	srv := &http.Server{
		Handler: corsMiddleware(r),
		Addr:    address,
	}

	_, port := getHostAndPortFromAddress(address)
	if port == 0 {
		return errors.New("invalid address to bind")
	}

	autodiscovery, err := newAutodiscoveryServer(autodiscoveryServiceName, port)
	if err != nil {
		return fmt.Errorf("unable to register autodiscovery server for myinstants: %w", err)
	}
	defer autodiscovery.Shutdown()

	slog.Info("listening for http connections", "address", address)
	slog.Info("registering autodiscovery server", "service", autodiscoveryServiceName)
	return srv.ListenAndServe()
}

type response struct {
	Label   string      `json:"label,omitempty"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// languageFor negotiates the response language from an optional
// Accept-Language header, defaulting to English. The UI never sends this
// header — it already translates by label on its own — so this exists for
// any other client.
func languageFor(r *http.Request) language.Tag {
	if header := r.Header.Get("Accept-Language"); header != "" {
		return i18n.MatchAcceptLanguage(header)
	}

	return i18n.Supported[0]
}

func writeErrorMessage(w http.ResponseWriter, status int, lang language.Tag, label string) {
	out := &response{
		Label:   label,
		Message: i18n.Text(lang, label),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(out); err != nil {
		http.Error(w, "unknown error", http.StatusInternalServerError)
	}
}

func writeSuccessResponse(w http.ResponseWriter, data interface{}) {
	out := &response{
		Data: data,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		http.Error(w, "unknown error", http.StatusInternalServerError)
	}
}

type botPlayRequest struct {
	URL string `json:"url,omitempty"`
}

type botPlayResponse struct {
	ExitReason string `json:"exitReason,omitempty"`
}

func (s *Server) handleBotPlay(w http.ResponseWriter, r *http.Request) {
	lang := languageFor(r)

	in := new(botPlayRequest)
	if err := json.NewDecoder(r.Body).Decode(in); err != nil {
		writeErrorMessage(w, http.StatusBadRequest, lang, "invalid_body")
		return
	}

	if !s.bot.Status().Connected {
		writeErrorMessage(w, http.StatusConflict, lang, "bot_not_connected")
		return
	}

	exitReason, err := s.player.Play(in.URL)
	switch err {
	case nil:
		// continue
	case instant.ErrInvalidLink:
		writeErrorMessage(w, http.StatusBadRequest, lang, "invalid_url")
		return
	case fsutil.ErrNotFound:
		writeErrorMessage(w, http.StatusNotFound, lang, "instant_not_found")
		return
	case fsutil.ErrUnsuportedAudioFormat:
		writeErrorMessage(w, http.StatusUnprocessableEntity, lang, "unsuported_audio_format")
		return
	default:
		writeErrorMessage(w, http.StatusInternalServerError, lang, "unknown_error")
		return
	}

	writeSuccessResponse(w, &botPlayResponse{exitReason})
}

func (s *Server) handleBotStop(w http.ResponseWriter, r *http.Request) {
	s.player.Stop()
}

type botStatusResponse struct {
	Connected   bool   `json:"connected"`
	GuildID     string `json:"guildId,omitempty"`
	GuildName   string `json:"guildName,omitempty"`
	ChannelID   string `json:"channelId,omitempty"`
	ChannelName string `json:"channelName,omitempty"`
}

func (s *Server) handleBotStatus(w http.ResponseWriter, r *http.Request) {
	status := s.bot.Status()
	out := &botStatusResponse{Connected: status.Connected}
	if status.Connected {
		out.GuildID, out.GuildName = status.GuildID.String(), status.GuildName
		out.ChannelID, out.ChannelName = status.ChannelID.String(), status.ChannelName
	}
	writeSuccessResponse(w, out)
}

func (s *Server) handleInstantContent(w http.ResponseWriter, r *http.Request) {
	lang := languageFor(r)

	url := r.PathValue("url")
	if !instant.IsLinkValid(url) {
		writeErrorMessage(w, http.StatusBadRequest, lang, "invalid_url")
		return
	}

	info, err := instant.GetPlayable(url)
	if err != nil {
		writeErrorMessage(w, http.StatusInternalServerError, lang, "unknown_error")
		return
	}

	writeSuccessResponse(w, info)
}

type instantButton struct {
	Name string `json:"name,omitempty"`
	URL  string `json:"url,omitempty"`
}

type instantListResponse struct {
	Instants []*instantButton `json:"instants"`
	Pages    int              `json:"pages"`
}

// pageSize is how many instants myinstants.com puts on a full page. Their pages
// no longer carry a pager, so the page count is inferred from it: a full page
// means there may be more, a short one is the last.
const pageSize = 36

const defaultRegion = "us"

var (
	errNameLinkMismatch = errors.New("names and links count do not match")

	// playURLPattern captures the clip path from a play button's
	// onclick="play('/media/sounds/x.mp3', 'loader-…', '…')".
	playURLPattern = regexp.MustCompile(`play\(\s*'([^']+)'`)

	// regionPattern guards the region before it is put into an upstream path.
	regionPattern = regexp.MustCompile(`^[a-z]{2}$`)
)

// totalPages infers the page count from how many instants the requested page
// held.
func totalPages(page, count int) int {
	switch {
	case count >= pageSize:
		return page + 1
	case count > 0:
		return page
	default:
		return max(1, page-1)
	}
}

// parseInstantList turns a myinstants.com listing page into the API response.
// Split out of handleListInstants so the scraping — the part most likely to break
// when their markup changes — can be tested against a fixture instead of the
// live site.
func parseInstantList(r io.Reader, baseURL string, page int) (*instantListResponse, error) {
	document, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, err
	}

	names := []string{}
	links := []string{}

	document.Find(".instant-link").Each(func(i int, anchor *goquery.Selection) {
		names = append(names, anchor.Text())
	})

	document.Find(".small-button").Each(func(i int, button *goquery.Selection) {
		onclick, ok := button.Attr("onclick")
		if !ok {
			return
		}

		match := playURLPattern.FindStringSubmatch(onclick)
		if match == nil {
			return
		}

		links = append(links, baseURL+match[1])
	})

	if len(names) != len(links) {
		return nil, errNameLinkMismatch
	}

	instants := []*instantButton{}
	for i, name := range names {
		instants = append(instants, &instantButton{
			Name: name,
			URL:  links[i],
		})
	}

	return &instantListResponse{
		Instants: instants,
		Pages:    totalPages(page, len(instants)),
	}, nil
}

func (s *Server) handleListInstants(w http.ResponseWriter, r *http.Request) {
	lang := languageFor(r)
	vars := r.URL.Query()

	region := strings.ToLower(strings.TrimSpace(vars.Get("region")))
	if region == "" {
		region = defaultRegion
	}
	if !regionPattern.MatchString(region) {
		writeErrorMessage(w, http.StatusBadRequest, lang, "invalid_region")
		return
	}

	// The UI sends page=undefined when it has no page, so anything that is not
	// a positive number means the first page rather than an error.
	page, err := strconv.Atoi(strings.TrimSpace(vars.Get("page")))
	if err != nil || page < 1 {
		page = 1
	}

	// A search goes to /search/, which ignores the region. Browsing without one
	// goes to the region's index: /search/ with no name answers 404.
	var url string
	search := strings.Replace(strings.TrimSpace(vars.Get("search")), " ", "+", -1)
	if search != "" {
		url = s.baseURL() + "/search/?page=" + strconv.Itoa(page) + "&name=" + search
	} else {
		url = s.baseURL() + "/en/index/" + region + "/?page=" + strconv.Itoa(page)
	}

	response, err := s.httpClient().Get(url)
	if err != nil {
		slog.Error("http.Get", "err", err)
		writeErrorMessage(w, http.StatusBadGateway, lang, "http_request")
		return
	}
	defer response.Body.Close()

	switch response.StatusCode {
	case http.StatusOK:
		// continue
	case http.StatusNotFound:
		writeSuccessResponse(w, &instantListResponse{
			Instants: []*instantButton{},
			Pages:    totalPages(page, 0),
		})
		return
	default:
		slog.Error("Bad http status", "StatusCode", response.StatusCode)
		writeErrorMessage(w, http.StatusBadGateway, lang, "bad_http_status")
		return
	}

	list, err := parseInstantList(response.Body, s.baseURL(), page)
	switch err {
	case nil:
		// continue
	case errNameLinkMismatch:
		writeErrorMessage(w, http.StatusInternalServerError, lang, "name_link_not_matched")
		return
	default:
		slog.Error("parseInstantList", "err", err)
		writeErrorMessage(w, http.StatusInternalServerError, lang, "unknown_error")
		return
	}

	writeSuccessResponse(w, list)
}
