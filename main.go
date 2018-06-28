package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const contentType = "application/x-www-form-urlencoded; charset=UTF-8"
const gameURLPrefix = "https://community.steam-api.com/ITerritoryControlMinigameService"
const activePlanetsEndpoint = "https://community.steam-api.com/ITerritoryControlMinigameService/GetPlanets/v0001/?active_only=1&language=schinese"
const leaveGameEndpoint = "https://community.steam-api.com/IMiniGameService/LeaveGame/v0001/"

var planetZoneBlacklist = PlanetZoneBlackList{blacklist: make(map[string]map[int]bool)}

type PlanetZoneBlackList struct {
	mutex     sync.RWMutex
	blacklist map[string]map[int]bool
}

func (p *PlanetZoneBlackList) Add(planet string, zonePosition int) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if zoneMap, found := p.blacklist[planet]; found {
		zoneMap[zonePosition] = true
	} else {
		p.blacklist[planet] = map[int]bool{zonePosition: true}
	}
}

func (p *PlanetZoneBlackList) IsBlacklisted(planet string, zonePosition int) bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	if zoneMap, found := p.blacklist[planet]; found {
		return zoneMap[zonePosition]
	}
	return false
}

func getBestAvailablePlanet() (*Planet, error) {
	res, err := http.Get(activePlanetsEndpoint)
	if err != nil {
		return nil, err
	}
	buf := struct{ Response struct{ Planets []Planet } }{}
	err = json.NewDecoder(res.Body).Decode(&buf)
	if err != nil {
		return nil, errors.New("[Connection Fail]Invalid response received when getting planets")
	}
	var bestDifficulty int
	var choosen Planet
	for _, p := range buf.Response.Planets {
		if pd := getBestAvailableDifficulty(&p); pd > bestDifficulty {
			choosen = p
			bestDifficulty = pd
		} else if bestDifficulty > 0 && pd == bestDifficulty && p.State.Progress < choosen.State.Progress {
			choosen = p
		}
	}
	if bestDifficulty == 0 {
		return nil, errors.New("No avaliable planet right now")
	}
	return &choosen, nil
}

func getBestAvailableDifficulty(p *Planet) int {
	if !p.State.Active || p.State.Captured {
		return 0
	}
	planet, err := getPlanetInfo(p.ID)
	for err != nil {
		log.Println("[SalienBot] Failed updating plannet " + p.State.Name + " difficulty, retry in 2s...")
		time.Sleep(2 * time.Second)
		planet, err = getPlanetInfo(p.ID)
	}

	bestDifficulty := 0
	for _, z := range planet.Zones {
		if !planetZoneBlacklist.IsBlacklisted(p.ID, z.Position) && z.Difficulty > bestDifficulty {
			bestDifficulty = z.Difficulty
		}
	}
	return bestDifficulty
}

type AccountHandler struct {
	steamToken           string
	logger               *log.Logger
	roundCounter         uint64
	bestPlanet           *Planet
	lastBestPlanetUpdate time.Time
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

func chooseZone(p *Planet) (*Zone, error) {
	var z Zone
	for _, zone := range p.Zones {
		if !zone.Captured &&
			!planetZoneBlacklist.IsBlacklisted(p.ID, zone.Position) &&
			(z.GameID == "" || zone.Difficulty >= z.Difficulty) {
			z = zone
		}
	}
	if z.GameID == "" {
		return nil, errors.New("No available zone in the planet")
	}
	return &z, nil
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
		score = "600"
	case 2:
		score = "1200"
	case 3:
		score = "2400"
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
	if player.TimeInZone < 110 {
		waitSeconds := 110 - player.TimeInZone
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
	res, err := http.Post(leaveGameEndpoint+"?access_token="+acc.steamToken+"&gameid="+gameID, contentType, bytes.NewBuffer([]byte("")))
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

func (acc *AccountHandler) zoneJoinHandle(nextZone *Zone, player *Player, planet *Planet) error {
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
			return errors.New("Already in a game while trying to join a zone ")
		}
	}
	planetZoneBlacklist.Add(planet.ID, nextZone.Position)
	return errors.New("Zone (" + planet.State.Name + "-" + strconv.Itoa(nextZone.Position) + ") is potentially full, and now be blacklisted.")
}

func (acc *AccountHandler) updateBestPlanet() {
	if time.Since(acc.lastBestPlanetUpdate) < 30*time.Second {
		return
	}
	bestPlanet, err := getBestAvailablePlanet()
	if err != nil {
		acc.logger.Println("bestPlanet update failed" + err.Error() + ", will retry in the next round")
		return
	}
	acc.lastBestPlanetUpdate = time.Now()

	currentDifficulty := getBestAvailableDifficulty(acc.bestPlanet)
	bestDifficulty := getBestAvailableDifficulty(bestPlanet)

	if currentDifficulty < bestDifficulty {
		acc.logger.Printf("Best Planet Changed: %s (%d) -> %s (%d, %.2f%%) \n",
			acc.bestPlanet.State.Name,
			currentDifficulty,
			bestPlanet.State.Name,
			bestDifficulty,
			bestPlanet.State.Progress*100)
		acc.bestPlanet = bestPlanet
	}
}

func (acc *AccountHandler) round() error {
	acc.roundCounter++
	acc.logger.Printf("=== Round %d ===\n", acc.roundCounter)
	acc.updateBestPlanet()
	player, err := acc.getPlayerInfo()
	if err != nil {
		return err
	}

	planetID := player.ActivePlanet
	if planetID == "" {
		acc.logger.Println("Not in a planet, Joining planet " + acc.bestPlanet.State.Name + "...")
		err = acc.joinPlanet(acc.bestPlanet)
		if err != nil {
			return err
		}
		planetID = acc.bestPlanet.ID
	}
	planet, err := getPlanetInfo(planetID)
	if err != nil {
		return err
	}
	if acc.bestPlanet.ID != planetID || planet.State.Captured || !planet.State.Active {
		if acc.bestPlanet.ID != planetID {
			acc.logger.Println("A better planet " + acc.bestPlanet.State.Name + " is available, leaving " + planet.State.Name + "...")
		} else {
			acc.logger.Println("Planet " + planet.State.Name + " is inactive or already captured, leaving...")
		}
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
		nextZone, err := chooseZone(planet)
		if err != nil {
			acc.logger.Println(err.Error(), ", leaving plannet")
			if err = acc.leaveGame(player.ActivePlanet); err != nil {
				return err
			}
			return errors.New("Planet reset")
		}
		acc.logger.Printf("Joining Zone:%d(%d %.2f%%)...\n",
			nextZone.Position,
			nextZone.Difficulty,
			nextZone.CaptureProgress*100)

		err = acc.zoneJoinHandle(nextZone, player, planet)
		if err != nil {
			return err
		}
		waitSeconds := 110
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
	err := errors.New("InitAccountError")
	var bestPlanet *Planet
	for err != nil {
		bestPlanet, err = getBestAvailablePlanet()
		time.Sleep(2 * time.Second)
	}

	return &AccountHandler{
		steamToken:           token,
		logger:               log.New(os.Stdout, "SalienBot|"+token[:6]+"|", log.Ltime),
		lastBestPlanetUpdate: time.Now(),
		bestPlanet:           bestPlanet,
	}
}

func (acc *AccountHandler) Start() {
	go func() {
		for {
			waitTime := 2 * time.Second
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
		log.Println("[SalienBot] 0.1.3 - Listening to terminate signal ctrl-c...")
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		errc <- fmt.Errorf("Signal %v", <-c)
	}()

	for _, token := range strings.Split(steamTokens, ",") {
		NewAccountHandler(token).Start()
		time.Sleep(3 * time.Second)
	}

	log.Println("[SalienBot] 0.1.3 -  Terminated - ", <-errc)
}
