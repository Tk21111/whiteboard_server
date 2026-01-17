package main

import (
	"encoding/json"
	"flag"
	"log"
	"math/rand"
	"time"

	"github.com/gorilla/websocket"
)

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	P float64 `json:"p"`
}

type Transform struct {
	X   float64 `json:"x"`
	Y   float64 `json:"y"`
	Rot float64 `json:"rot"`
	W   float64 `json:"w"`
	H   float64 `json:"h"`
}

type Msg struct {
	Operation string      `json:"operation"`
	ID        string      `json:"id,omitempty"`
	Payload   interface{} `json:"payload,omitempty"`
	Transform *Transform  `json:"transform,omitempty"`
	Points    []Point     `json:"points,omitempty"`
	Clock     int64       `json:"clock,omitempty"`
}

func randFloat(min, max float64) float64 {
	return min + rand.Float64()*(max-min)
}

func randomTransform() *Transform {
	return &Transform{
		X:   randFloat(-1000, 1000),
		Y:   randFloat(-1000, 1000),
		Rot: randFloat(0, 6.28),
		W:   50,
		H:   50,
	}
}

func randomPoints(n int) []Point {
	pts := make([]Point, n)
	for i := range pts {
		pts[i] = Point{
			X: randFloat(0, 2000),
			Y: randFloat(0, 2000),
			P: 0.5,
		}
	}
	return pts
}

func main() {
	var (
		wsURL    = flag.String("url", "ws://localhost:8080/ws?roomId=test", "ws url")
		rate     = flag.Int("rate", 200, "messages per second")
		duration = flag.Int("duration", 10, "seconds")
		domID    = flag.String("dom", "bomb-dom", "dom id")
	)
	flag.Parse()

	conn, _, err := websocket.DefaultDialer.Dial(*wsURL, nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer conn.Close()

	log.Println("🔥 Connected to", *wsURL)

	ticker := time.NewTicker(time.Second / time.Duration(*rate))
	defer ticker.Stop()

	end := time.After(time.Duration(*duration) * time.Second)

	var clock int64

	for {
		select {
		case <-end:
			log.Println("💥 Bombardment finished")
			return

		case <-ticker.C:
			clock++

			msgs := []Msg{
				{
					Operation: "dom-transform",
					ID:        *domID,
					Transform: randomTransform(),
					Clock:     clock,
				},
				{
					Operation: "dom-payload",
					ID:        *domID,
					Payload: map[string]any{
						"text": rand.Intn(100000),
					},
					Clock: clock,
				},
				{
					Operation: "stroke-update",
					ID:        "bomb-stroke",
					Points:    randomPoints(3),
					Clock:     clock,
				},
			}

			data, _ := json.Marshal(msgs)
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				log.Println("write error:", err)
				return
			}
		}
	}
}
