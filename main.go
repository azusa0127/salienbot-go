package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"
)

const contentType = "application/json"
const gameUrlPrefix = "https://community.steam-api.com/ITerritoryControlMinigameService"
const activePlanetsEndpoint = "https://community.steam-api.com/ITerritoryControlMinigameService/GetPlanets/v0001/?active_only=1&language=schinese"
const leaveGameEndpoint = "https://community.steam-api.com/IMiniGameService/LeaveGame/v0001/"

var steamToken string
var rng = rand.New(rand.NewSource(time.Now().UnixNano()))

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

	res, err := http.Get(gameUrlPrefix + "/GetPlanet/v0001/?id=" + planetID + "&language=schinese")
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

func getPlayerInfo() (*Player, error) {
	res, err := http.Post(gameUrlPrefix+"/GetPlayerInfo/v0001/?access_token="+steamToken, contentType, bytes.NewBuffer(nil))
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

func joinZone(zone *Zone) error {
	res, err := http.Post(gameUrlPrefix+"/JoinZone/v0001/?zone_position="+strconv.Itoa(zone.Position)+"&access_token="+steamToken, contentType, bytes.NewBuffer(nil))
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

func submitScore(zone *Zone) (string, error) {
	// Validate planet change
	if p, err := getPlayerInfo(); err != nil {
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
		gameUrlPrefix+"/ReportScore/v0001/?score="+score+"&access_token="+steamToken, contentType, bytes.NewBuffer(nil))

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

func existingGameHandle(player *Player, zones []Zone) (string, error) {
	if player.ActiveZoneGame == "" {
		return "", nil
	}
	log.Printf("Already in game zone %s for %d seconds, trying to recover...\n", player.ActiveZonePosition, player.TimeInZone)
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
	if player.TimeInZone < 115 {
		waitSeconds := 115 - player.TimeInZone
		log.Printf("Submitting score for zone %d(%d %.2f%%) in %d seconds...\n",
			target.Position,
			target.Difficulty,
			target.CaptureProgress*100,
			waitSeconds)
		time.Sleep(time.Duration(waitSeconds) * time.Second)
	}
	return submitScore(target)
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

func joinPlanet(p *Planet) error {
	log.Println("Joining planet " + p.State.Name)
	res, err := http.Post(gameUrlPrefix+"/JoinPlanet/v0001/?id="+p.ID+"&access_token="+steamToken, contentType, bytes.NewBuffer(nil))
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

func leavePlanet(planetID string) error {
	res, err := http.Post(leaveGameEndpoint+"?gameid="+planetID+"&access_token="+steamToken, contentType, bytes.NewBuffer(nil))
	if err != nil {
		return err
	}
	buf := struct{ Response *interface{} }{}
	err = json.NewDecoder(res.Body).Decode(&buf)
	if err != nil {
		return errors.New("[Connection Fail]Invalid response received when leaving planet")
	}
	if buf.Response == nil {
		return errors.New("Failed Leaving Planet " + planetID)
	}
	return nil
}

func round() error {
	log.Println("=== New Start ===")
	player, err := getPlayerInfo()
	if err != nil {
		return err
	}
	planetID := player.ActivePlanet
	if planetID == "" {
		log.Println("Not in a planet, finding a new one to join...")
		p, err := getBestAvaliablePlanet()
		if err != nil {
			return err
		}
		err = joinPlanet(p)
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
		log.Println("Planet " + planet.State.Name + " is inactive or already captured, leaving...")
		if err = leavePlanet(planetID); err != nil {
			return err
		}
		return errors.New("Leaved planet " + planet.State.Name + " ...")
	}

	if !(planet.State.Active && planet.State.Progress < 1.0) {
		log.Fatal("[FATAL] Planet is not active or already been captured.")
	}

	log.Printf("Planet:%s|Progress:%.2f%%|Level:%d|Exp:%s/%s\n",
		planet.State.Name,
		planet.State.Progress*100,
		player.Level,
		player.Score,
		player.NextLevelScore)
	newScore, err := existingGameHandle(player, planet.Zones)
	if err != nil {
		return err
	}
	if newScore == "" {
		nextZone := chooseZone(planet.Zones)
		log.Printf("Joining Zone:%d(%d %.2f%%)...\n",
			nextZone.Position,
			nextZone.Difficulty,
			nextZone.CaptureProgress*100)

		err = joinZone(nextZone)
		if err != nil {
			return err
		}
		waitSeconds := 120 - rng.Intn(11)
		log.Printf("...Joined! wait %ds to submit.\n", waitSeconds)
		time.Sleep(time.Duration(waitSeconds) * time.Second)

		newScore, err = submitScore(nextZone)
		if err != nil {
			return err
		}
	}

	log.Printf("=== SUCCUSS (%s -> %s) ===\n", player.Score, newScore)
	return nil
}

func main() {
	flag.StringVar(&steamToken, "token", os.Getenv("STEAM_TOKEN"), "Steam token value from https://steamcommunity.com/saliengame/gettoken")
	if steamToken == "" {
		log.Fatal("[STEAM_TOKEN MISSING] Please set env STEAM_TOKEN first")
	}
	log.SetPrefix("SalienBot|" + steamToken[:6] + "|")
	log.SetFlags(log.Ltime)

	for {
		waitTime := time.Duration(2+rng.Intn(3)) * time.Second
		err := round()
		if err != nil {
			log.Println("[ERROR]", err.Error())
			log.Println("Retry in 8 second.")
			waitTime = 8 * time.Second
		}
		time.Sleep(waitTime)
	}
}
