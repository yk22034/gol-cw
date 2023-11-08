package gol

import (
	"uk.ac.bris.cs/gameoflife/util"
)

func getNumNeighbours(y, x int, world func(y, x int) uint8, p Params) int {
	numNeighbours := 0
	for yInc := -1; yInc <= 1; yInc++ {
		var testY int = (y + yInc + p.ImageHeight) % p.ImageHeight
		for xInc := -1; xInc <= 1; xInc++ {
			var testX int = (x + xInc + p.ImageWidth) % p.ImageWidth
			if (testX != x || testY != y) && world(testY, testX) == 255 {
				numNeighbours++
			}
		}
	}

	return numNeighbours
}

func setCell(y, x int, world func(y, x int) uint8, newValue uint8, events chan<- Event, turn int) {
	if world(y, x) != newValue {
		events <- CellFlipped{CompletedTurns: turn, Cell: util.Cell{X: x, Y: y}}
	}
}

func worker(y1, y2 int, world func(y, x int) uint8, events chan<- Event, c chan<- [][]uint8, p Params, turn int) {
	sliceHeight := (y2 - y1) + 1
	var newSlice = make([][]uint8, sliceHeight)
	for i := 0; i < sliceHeight; i++ {
		newSlice[i] = make([]uint8, p.ImageWidth)
	}
	for y := y1; y <= y2; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			neighbours := getNumNeighbours(y, x, world, p)
			cellValue := world(y, x)
			switch {
			case neighbours < 2:
				setCell(y, x, world, 0, events, turn)
				newSlice[y-y1][x] = 0
			case neighbours > 3 && cellValue == 255:
				setCell(y, x, world, 0, events, turn)
				newSlice[y-y1][x] = 0
			case neighbours == 3 && cellValue == 0:
				setCell(y, x, world, 255, events, turn)
				newSlice[y-y1][x] = 255
			default:
				newSlice[y-y1][x] = cellValue
			}
		}
	}
	c <- newSlice
}
