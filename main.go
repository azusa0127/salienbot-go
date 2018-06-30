package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
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
var bestAvailablePlanet = BestAvailablePlanet{}

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

type BestAvailablePlanet struct {
	mutex        sync.Mutex
	ttl          time.Duration
	bestPlanetID string
	difficulty   uint
	lastUpdateAt time.Time
}

func (b *BestAvailablePlanet) Get() (string, uint, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if time.Since(b.lastUpdateAt) >= b.ttl {
		bestPlanetID, difficulty, err := getBestAvailablePlanet()
		if err != nil {
			return "", 0, err
		}
		b.bestPlanetID = bestPlanetID
		b.difficulty = difficulty
		b.lastUpdateAt = time.Now()
	}
	return b.bestPlanetID, b.difficulty, nil
}

func getBestAvailablePlanet() (string, uint, error) {
	res, err := http.Get(activePlanetsEndpoint)
	if err != nil {
		return "", 0, err
	}
	buf := struct{ Response struct{ Planets []Planet } }{}
	err = json.NewDecoder(res.Body).Decode(&buf)
	if err != nil {
		return "", 0, errors.New("[Connection Fail]Invalid response received when getting planets")
	}

	log.Printf("[SalienBot] Planets Info:\n")
	var bestPlanet Planet
	var bestDifficulty uint
	for _, p := range buf.Response.Planets {
		pd := getBestAvailableDifficulty(p)
		log.Printf("  %s (%d) - %.2f%%\n", p.State.Name, pd, p.State.Progress*100)
		if pd > bestDifficulty {
			bestDifficulty = pd
			bestPlanet = p
		} else if pd == bestDifficulty {
			if bestPlanet.State.Progress > p.State.Progress || pd == 1 {
				bestPlanet = p
			}
		}
	}
	log.Printf("  (Best Planet) %s (%d) - %.2f%%\n", bestPlanet.State.Name, bestDifficulty, bestPlanet.State.Progress*100)
	return bestPlanet.ID, bestDifficulty, nil
}

func getBestAvailableDifficulty(p Planet) uint {
	if !p.State.Active || p.State.Captured {
		return 0
	}
	planet, err := getPlanetInfo(p.ID)
	for err != nil {
		log.Println("[SalienBot] Failed getting plannet " + p.State.Name + " info, retry in 2s...")
		time.Sleep(2 * time.Second)
		planet, err = getPlanetInfo(p.ID)
	}

	var bestDifficulty int
	for _, z := range planet.Zones {
		if !planetZoneBlacklist.IsBlacklisted(planet.ID, z.Position) && !z.Captured && z.Difficulty > bestDifficulty {
			bestDifficulty = z.Difficulty
		}
	}
	return uint(bestDifficulty)
}

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
		acc.logger.Printf("Submitting score(%.f) for zone %d(%d %.2f%%) in %d seconds...\n",
			600*(math.Exp2(float64(target.Difficulty-1))),
			target.Position,
			target.Difficulty,
			target.CaptureProgress*100,
			waitSeconds)
		time.Sleep(time.Duration(waitSeconds) * time.Second)
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

func (acc *AccountHandler) zoneJoinHandle(nextZone *Zone, planet *Planet) error {
	retry := 0
	for retry < 3 {
		retry++
		if err := acc.joinZone(nextZone); err == nil {
			return nil
		}
		acc.logger.Println("Zone Join Failed [RETRY in 5s]")
		time.Sleep(5 * time.Second)
		if p, err := acc.getPlayerInfo(); err != nil {
			return err
		} else if p.ActiveZoneGame != "" {
			return errors.New("Already in a game while trying to join a zone ")
		}
	}
	planetZoneBlacklist.Add(planet.ID, nextZone.Position)
	return errors.New("Zone (" + planet.State.Name + "-" + strconv.Itoa(nextZone.Position) + ") is potentially full, and now be blacklisted.")
}

func (acc *AccountHandler) round() error {
	acc.roundCounter++
	acc.logger.Printf("=== Round %d ===\n", acc.roundCounter)
	var player *Player
	var err error
	if player, err = acc.getPlayerInfo(); err != nil {
		return err
	}

	if player.TimeInZone > 140 {
		acc.logger.Printf("Stucking in a game for %d seconds, trying to reset...\n", player.TimeInZone)
		if err := acc.leaveGame(player.ActiveZoneGame); err != nil {
			return err
		}
		return errors.New("Game timed-out")
	}
	bestPlanetID, bestDifficulty, err := bestAvailablePlanet.Get()
	if err != nil {
		return err
	}

	var planet *Planet
	if player.ActivePlanet == "" {
		planet, err = getPlanetInfo(bestPlanetID)
		if err != nil {
			return err
		}
		acc.logger.Println("Not in a planet, Joining planet " + planet.State.Name + "...")
		if err = acc.joinPlanet(planet); err != nil {
			return err
		}
	} else {
		planet, err = getPlanetInfo(player.ActivePlanet)
		if err != nil {
			return err
		}
	}

	if planet.State.Captured || !planet.State.Active {
		acc.logger.Println("Planet " + planet.State.Name + " is inactive or already captured, leaving...")
		if err := acc.leaveGame(planet.ID); err != nil {
			return err
		}
		return errors.New("Leaved planet " + planet.State.Name + " ...")
	}

	if bestPlanetID != planet.ID {
		acc.logger.Printf("A better planet with difficulty %d is available, leaving %s ...\n", bestDifficulty, planet.State.Name)
		if player.ActiveZoneGame != "" {
			acc.leaveGame(player.ActiveZoneGame)
		}
		return errors.New("Leaved planet " + planet.State.Name + " for a better planet...")
	}

	acc.logger.Printf("Planet:%s|Progress:%.2f%%|Level:%d|Exp:%s/%s\n",
		planet.State.Name,
		planet.State.Progress*100,
		player.Level,
		player.Score,
		player.NextLevelScore)
	var newScore string
	if newScore, err = acc.existingGameHandle(player, planet.Zones); err != nil {
		return err
	}
	if newScore == "" {
		var nextZone *Zone
		if nextZone, err = chooseZone(planet); err != nil {
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

		if err = acc.zoneJoinHandle(nextZone, planet); err != nil {
			return err
		}
		waitSeconds := 110
		acc.logger.Printf("...Joined! wait %ds to submit.\n", waitSeconds)
		time.Sleep(time.Duration(waitSeconds) * time.Second)

		if newScore, err = acc.submitScore(nextZone); err != nil {
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
		log.Println("[SalienBot] 0.2.1 Listening to terminate signal ctrl-c...")
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		errc <- fmt.Errorf("Signal %v", <-c)
	}()

	if _, _, err := bestAvailablePlanet.Get(); err != nil {
		log.Fatal(err)
	}

	for _, token := range strings.Split(steamTokens, ",") {
		NewAccountHandler(token).Start()
		time.Sleep(3 * time.Second)
	}

	log.Println("[SalienBot] 0.2.1 Terminated - ", <-errc)
}
