package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

const contentType = "application/json"
const gameUrlPrefix = "https://community.steam-api.com/ITerritoryControlMinigameService"

var steamToken = os.Getenv("STEAM_TOKEN")

type Planet struct {
	State struct {
		Name     string
		Active   bool
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
		return nil, err
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
		return nil, err
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
		return err
	}
	if buf.Response.ZoneInfo == nil {
		return errors.New("Failed Joining Zone")
	}
	return nil
}

func submitScore(zone *Zone) (string, error) {
	// Validate planet change
	if p, _ := getPlayerInfo(); p.ActiveZoneGame == "" {
		return "", errors.New("Planet changed, retry in 5")
	}
	var score string
	switch zone.Difficulty {
	case 1:
		score = "595"
	case 2:
		score = "1190"
	case 3:
		score = "2380"
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
		return "", err
	}
	if buf.Response.NewScore == "" {
		return "", errors.New("Failed Submitting the Score")
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
	if player.TimeInZone < 120 {
		waitSeconds := 120 - player.TimeInZone
		log.Printf("Submitting score for zone %d(%d %.2f%%) in %d seconds...\n",
			target.Position,
			target.Difficulty,
			target.CaptureProgress*100,
			waitSeconds)
		time.Sleep(time.Duration(waitSeconds) * time.Second)
	}
	return submitScore(target)
}

func round() error {
	log.Println("=== Starting a new round ===")
	player, err := getPlayerInfo()
	if err != nil {
		return err
	}

	if player.ActivePlanet == "" {
		log.Fatal("[FATAL] Must join a planet first.")
	}

	planet, err := getPlanetInfo(player.ActivePlanet)
	if err != nil {
		return err
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
		log.Printf("Joining NextZone:%d(%d %.2f%%)...\n",
			nextZone.Position,
			nextZone.Difficulty,
			nextZone.CaptureProgress*100)

		err = joinZone(nextZone)
		if err != nil {
			return err
		}
		log.Println("...Seccussfull! Wait for 120s to submit score.")

		time.Sleep(120 * time.Second)

		newScore, err = submitScore(nextZone)
		if err != nil {
			return err
		}
	}

	log.Printf("=== Score Submitted Successfully (%s -> %s) ===\n", player.Score, newScore)
	return nil
}

func main() {
	if steamToken == "" {
		log.Fatal("[STEAM_TOKEN MISSING] Please set env STEAM_TOKEN first")
	}
	log.SetPrefix("SalienBot|" + steamToken[:6] + "|")
	log.SetFlags(log.Ltime)

	for {
		waitTime := 1 * time.Second
		err := round()
		if err != nil {
			log.Println("[ERROR]", err.Error())
			log.Println("Retry in 5 second.")
			waitTime = 5 * time.Second
		}
		time.Sleep(waitTime)
	}
}
