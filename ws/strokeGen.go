package ws

import (
	"math/rand/v2"

	"github.com/Tk21111/whiteboard_server/config"
	"github.com/Tk21111/whiteboard_server/middleware"
	"github.com/google/uuid"
)

func genStrokeStart(id string) []byte {
	msg := []config.ServerMsg{
		{
			Clock: 0,
			Payload: config.NetworkMsg{
				Operation: "stroke-start",
				ID:        id,
				Stroke: &config.StrokeObjectInterface{
					ID:        id,
					Kind:      "stroke",
					Color:     "rgb(0,0,0)",
					Operation: "draw",
					Opacity:   1,
					Size:      3,
					Points: []config.Point{
						{X: rand.Float64() * 1920, Y: rand.Float64() * 1080, P: 0.5},
					},
				},
			},
		},
	}
	return middleware.EncodeNetworkMsg(msg)
}

func genStrokeUpdate(id string, n int) []byte {
	points := make([]config.Point, n)
	for i := range points {
		points[i] = config.Point{
			X: rand.Float64() * 1920,
			Y: rand.Float64() * 1080,
			P: 0.5,
		}
	}

	msg := []config.ServerMsg{
		{
			Payload: config.NetworkMsg{
				Operation: "stroke-update",
				ID:        id,
				Points:    points,
			},
		},
	}
	return middleware.EncodeNetworkMsg(msg)
}

func genStrokeEnd(id string, clock int64) []byte {
	msg := []config.ServerMsg{
		{
			Clock: clock,
			Payload: config.NetworkMsg{
				Operation: "stroke-end",
				ID:        id,
			},
		},
	}
	return middleware.EncodeNetworkMsg(msg)
}

func BurnRoom(H *Hub, roomID string, strokes int, updatesPerStroke int) {
	go func() {
		for i := 0; i < strokes; i++ {

			id := uuid.NewString()

			H.Broadcast(roomID, genStrokeStart(id), nil)

			for u := 0; u < updatesPerStroke; u++ {
				H.Broadcast(roomID, genStrokeUpdate(id, 5+rand.IntN(20)), nil)
			}

			H.Broadcast(roomID, genStrokeEnd(id, int64(updatesPerStroke)), nil)
		}
	}()
}
