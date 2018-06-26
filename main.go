package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const contentType = "application/json"
const gameURLPrefix = "https://community.steam-api.com/ITerritoryControlMinigameService"
const activePlanetsEndpoint = "https://community.steam-api.com/ITerritoryControlMinigameService/GetPlanets/v0001/?active_only=1&language=schinese"
const leaveGameEndpoint = "https://community.steam-api.com/IMiniGameService/LeaveGame/v0001/"

var rng = rand.New(rand.NewSource(time.Now().UnixNano()))

type AccountHandler struct {
	steamToken   string
	logger       *log.Logger
	roundCounter uint64
}

type Planet struct {
	ID    string
	State struct {
		Name     string
		Active   bool
		Captured bool
		Progress float64 `json:"capture_progress"`
	}
	Zones []Zone
}
type Zone struct {
	Position        int     `json:"zone_position"`
	Captured        bool    `json:"captured"`
	CaptureProgress float64 `json:"capture_progress"`
	GameID          string
	Difficulty      int
	Type            int
}

func getPlanetInfo(planetID string) (*Planet, error) {
	buf := struct{ Response struct{ Planets []Planet } }{}

	res, err := http.Get(gameURLPrefix + "/GetPlanet/v0001/?id=" + planetID + "&language=schinese")
	if err != nil {
		return nil, err
	}
	err = json.NewDecoder(res.Body).Decode(&buf)
	if err != nil {
		return nil, errors.New("[Connection Fail]Invalid response received when getting planet info")
	}
	return &buf.Response.Planets[0], nil
}

type Player struct {
	Level              int
	ActivePlanet       string `json:"active_planet"`
	ActiveZoneGame     string `json:"active_zone_game"`
	ActiveZonePosition string `json:"active_zone_position"`
	TimeInZone         int    `json:"time_in_zone"`
	Score              string
	NextLevelScore     string `json:"next_level_score"`
}

func (acc *AccountHandler) getPlayerInfo() (*Player, error) {
	res, err := http.Post(gameURLPrefix+"/GetPlayerInfo/v0001/?access_token="+acc.steamToken, contentType, bytes.NewBuffer(nil))
	if err != nil {
		return nil, err
	}
	buf := struct{ Response Player }{}
	err = json.NewDecoder(res.Body).Decode(&buf)
	if err != nil {
		return nil, errors.New("[Connection Fail]Invalid response received when getting player info")
	}

	return &buf.Response, nil
}

func chooseZone(zones []Zone) *Zone {
	z := zones[0]
	for _, zone := range zones {
		if !zone.Captured && (z.Captured || zone.Difficulty >= z.Difficulty) {
			z = zone
		}
	}
	return &z
}

func (acc *AccountHandler) joinZone(zone *Zone) error {
	res, err := http.Post(gameURLPrefix+"/JoinZone/v0001/?zone_position="+strconv.Itoa(zone.Position)+"&access_token="+acc.steamToken, contentType, bytes.NewBuffer(nil))
	if err != nil {
		return err
	}
	buf := struct {
		Response struct {
			ZoneInfo *interface{} `json:"zone_info"`
		}
	}{}
	err = json.NewDecoder(res.Body).Decode(&buf)
	if err != nil {
		return errors.New("[Connection Fail]Invalid response received when joining zone")
	}
	if buf.Response.ZoneInfo == nil {
		return errors.New("Failed Joining Zone")
	}
	return nil
}

func (acc *AccountHandler) submitScore(zone *Zone) (string, error) {
	// Validate planet change
	if p, err := acc.getPlayerInfo(); err != nil {
		return "", err
	} else if p.ActiveZoneGame == "" {
		return "", errors.New("No Active Game found, possible planet changing in progress")
	}
	var score string
	switch zone.Difficulty {
	case 1:
		score = strconv.Itoa(600 - 5*rng.Intn(2))
	case 2:
		score = strconv.Itoa(1200 - 10*rng.Intn(2))
	case 3:
		score = strconv.Itoa(2400 - 20*rng.Intn(2))
	}

	res, err := http.Post(
		gameURLPrefix+"/ReportScore/v0001/?score="+score+"&access_token="+acc.steamToken, contentType, bytes.NewBuffer(nil))

	buf := struct {
		Response struct {
			NewScore string `json:"new_score"`
		}
	}{}
	err = json.NewDecoder(res.Body).Decode(&buf)
	if err != nil {
		return "", errors.New("[Connection Fail]Invalid response received when submitting score")
	}
	if buf.Response.NewScore == "" {
		return "", errors.New("Failed to submit the score")
	}
	return buf.Response.NewScore, nil
}

func (acc *AccountHandler) existingGameHandle(player *Player, zones []Zone) (string, error) {
	if player.ActiveZoneGame == "" {
		return "", nil
	}
	acc.logger.Printf("Already in game zone %s for %d seconds, trying to recover...\n", player.ActiveZonePosition, player.TimeInZone)
	zonePosition, err := strconv.Atoi(player.ActiveZonePosition)
	if err != nil {
		return "", errors.New("Invalid active_zone_position: " + "player.ActiveZonePosition")
	}
	var target *Zone
	for _, zone := range zones {
		if zone.Position == zonePosition {
			target = &zone
			break
		}
	}
	if player.TimeInZone < 112 {
		waitSeconds := 112 - player.TimeInZone
		acc.logger.Printf("Submitting score for zone %d(%d %.2f%%) in %d seconds...\n",
			target.Position,
			target.Difficulty,
			target.CaptureProgress*100,
			waitSeconds)
		time.Sleep(time.Duration(waitSeconds) * time.Second)
	} else if player.TimeInZone > 150 {
		acc.logger.Printf("Stucking in a game for %d seconds, trying to reset...\n", player.TimeInZone)
		if err = acc.leaveGame(player.ActiveZoneGame); err != nil {
			return "", err
		}
		return "", errors.New("Game timed-out")
	}
	return acc.submitScore(target)
}

func getBestAvaliablePlanet() (*Planet, error) {
	res, err := http.Get(activePlanetsEndpoint)
	if err != nil {
		return nil, err
	}
	buf := struct{ Response struct{ Planets []Planet } }{}
	err = json.NewDecoder(res.Body).Decode(&buf)
	if err != nil {
		return nil, errors.New("[Connection Fail]Invalid response received when getting planets")
	}
	var retVal *Planet
	for _, p := range buf.Response.Planets {
		if p.State.Active && !p.State.Captured && (retVal == nil || retVal.State.Progress > p.State.Progress) {
			retVal = &p
		}
	}
	if retVal == nil {
		return nil, errors.New("No avaliable planet right now")
	}
	return retVal, nil
}

func (acc *AccountHandler) joinPlanet(p *Planet) error {
	acc.logger.Println("Joining planet " + p.State.Name)
	res, err := http.Post(gameURLPrefix+"/JoinPlanet/v0001/?id="+p.ID+"&access_token="+acc.steamToken, contentType, bytes.NewBuffer(nil))
	if err != nil {
		return err
	}
	buf := struct{ Response *interface{} }{}
	err = json.NewDecoder(res.Body).Decode(&buf)
	if err != nil {
		return errors.New("[Connection Fail]Invalid response received when joining planet")
	}
	if buf.Response == nil {
		return errors.New("Failed Joining Planet " + p.ID)
	}
	return nil
}

func (acc *AccountHandler) leaveGame(gameID string) error {
	res, err := http.Post(leaveGameEndpoint+"?gameid="+gameID+"&access_token="+acc.steamToken, contentType, bytes.NewBuffer(nil))
	if err != nil {
		return err
	}
	buf := struct{ Response *interface{} }{}
	err = json.NewDecoder(res.Body).Decode(&buf)
	if err != nil {
		return errors.New("[Connection Fail]Invalid response received when leaving planet")
	}
	if buf.Response == nil {
		return errors.New("Failed Leaving Game " + gameID)
	}
	return nil
}

func (acc *AccountHandler) zoneJoinHandle(nextZone *Zone, player *Player) error {
	retry := 0
	for retry < 3 {
		retry++
		err := acc.joinZone(nextZone)
		if err == nil {
			return nil
		}
		acc.logger.Println("[RETRY in 5s]", err.Error())
		time.Sleep(5 * time.Second)
		player, err = acc.getPlayerInfo()
		if player.ActiveZoneGame != "" {
			return errors.New("Already in game while trying joinning a zone ")
		}
	}
	err := acc.leaveGame(player.ActivePlanet)
	if err != nil {
		return err
	}
	return errors.New("Zone failure reset")
}

func (acc *AccountHandler) round() error {
	acc.roundCounter++
	acc.logger.Printf("=== Round %d ===\n", acc.roundCounter)
	player, err := acc.getPlayerInfo()
	if err != nil {
		return err
	}
	planetID := player.ActivePlanet
	if planetID == "" {
		acc.logger.Println("Not in a planet, finding a new one to join...")
		p, err := getBestAvaliablePlanet()
		if err != nil {
			return err
		}
		err = acc.joinPlanet(p)
		if err != nil {
			return err
		}
		planetID = p.ID
	}
	planet, err := getPlanetInfo(planetID)
	if err != nil {
		return err
	}
	if planet.State.Captured || !planet.State.Active {
		acc.logger.Println("Planet " + planet.State.Name + " is inactive or already captured, leaving...")
		if err = acc.leaveGame(planetID); err != nil {
			return err
		}
		return errors.New("Leaved planet " + planet.State.Name + " ...")
	}

	if !(planet.State.Active && planet.State.Progress < 1.0) {
		acc.logger.Fatal("[FATAL] Planet is not active or already been captured.")
	}

	acc.logger.Printf("Planet:%s|Progress:%.2f%%|Level:%d|Exp:%s/%s\n",
		planet.State.Name,
		planet.State.Progress*100,
		player.Level,
		player.Score,
		player.NextLevelScore)
	newScore, err := acc.existingGameHandle(player, planet.Zones)
	if err != nil {
		return err
	}
	if newScore == "" {
		nextZone := chooseZone(planet.Zones)
		acc.logger.Printf("Joining Zone:%d(%d %.2f%%)...\n",
			nextZone.Position,
			nextZone.Difficulty,
			nextZone.CaptureProgress*100)

		err = acc.zoneJoinHandle(nextZone, player)
		if err != nil {
			return err
		}
		waitSeconds := 120 - rng.Intn(6)
		acc.logger.Printf("...Joined! wait %ds to submit.\n", waitSeconds)
		time.Sleep(time.Duration(waitSeconds) * time.Second)

		newScore, err = acc.submitScore(nextZone)
		if err != nil {
			return err
		}
	}

	acc.logger.Printf("=== Round %d Complete (%s -> %s) ===\n", acc.roundCounter, player.Score, newScore)
	return nil
}

func NewAccountHandler(token string) *AccountHandler {
	return &AccountHandler{
		steamToken: token,
		logger:     log.New(os.Stdout, "SalienBot|"+token[:6]+"|", log.Ltime),
	}
}

func (acc *AccountHandler) Start() {
	go func() {
		for {
			waitTime := time.Duration(2+rng.Intn(3)) * time.Second
			err := acc.round()
			if err != nil {
				acc.logger.Println("[ERROR]", err.Error(), "Retry in 8 second...")
				waitTime = 8 * time.Second
			}
			time.Sleep(waitTime)
		}
	}()
}

func main() {
	var steamTokens string
	flag.StringVar(&steamTokens, "token", os.Getenv("STEAM_TOKEN"), "Steam token value from https://steamcommunity.com/saliengame/gettoken")
	flag.Parse()
	if steamTokens == "" {
		log.Fatal("[SalienBot][STEAM_TOKEN MISSING] Please set env STEAM_TOKEN or passing in -token argument first")
	}
	errc := make(chan error)
	go func() {
		log.Println("[SalienBot] Listening terminate signal ctrl-c...")
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		errc <- fmt.Errorf("Signal %v", <-c)
	}()

	for _, token := range strings.Split(steamTokens, ",") {
		NewAccountHandler(token).Start()
		time.Sleep(3 * time.Second)
	}

	log.Println("[SalienBot] Terminated - ", <-errc)
}
