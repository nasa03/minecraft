package game

import (
	"math"
	"math/rand"

	"github.com/go-gl/mathgl/mgl32"
)

// TODO: persist game

// World holds the terrain, map and manages entity lifecycles.
type World struct {
	atlas *TextureAtlas

	// chunk map, provides lookup by location
	chunks VecMap[Chunk]

	// shader program that draws the chunks
	chunkShader *Shader

	// generates world terrain and content
	generator *WorldGenerator

	// queues tasks allowing defered processing
	tasks *TaskQueue
}

const (
	// dimensions
	ground    = 100.0
	bedrock   = 0.0
	maxHeight = 200.0

	// rendering
	visibleRadius     = 130.0
	destroyRadius     = 300.0
	playerSpawnRadius = 15

	// misc
	tasksPerFrame = 3
	seed          = 10
)

func newWorld(chunkShader *Shader, atlas *TextureAtlas) *World {
	w := &World{}
	w.chunkShader = chunkShader
	w.chunks = newVecMap[Chunk]()
	w.atlas = atlas
	w.generator = newWorldGenerator(seed)
	w.tasks = newQueue()
	return w
}

func (w *World) Init() {
	s := playerSpawnRadius
	for i := 0; i < s; i++ {
		for j := 0; j < s; j++ {
			p := mgl32.Vec3{float32(chunkWidth * i), 0, float32(chunkWidth * j)}
			w.SpawnChunk(p)
		}
	}
}

// Processes tasks queued.
func (w *World) ProcessTasks() {
	for i := 0; i < tasksPerFrame; i++ {
		f := w.tasks.Pop()
		if f == nil {
			break
		}
		f()
	}
}

// Spawns a new chunk at the given position.
// The param should a be a "valid" chunk position.
func (w *World) SpawnChunk(pos mgl32.Vec3) *Chunk {
	if int(pos.X())%chunkWidth != 0 ||
		int(pos.Y())%chunkHeight != 0 ||
		int(pos.Z())%chunkWidth != 0 {
		panic("invalid chunk position")
	}

	// init chunk, attribs, pointers and save
	chunk := newChunk(w.chunkShader, w.atlas, pos)
	w.chunks.Set(pos, chunk)
	s := w.generator.Terrain(chunk.pos)
	chunk.Init(s)

	w.tasks.Queue(func() {
		w.SpawnTrees(chunk)
		chunk.Buffer()
	})

	chunk.Buffer()
	return chunk
}

// Despawns the chunk and destroys the data on gpu.
func (w *World) DespawnChunk(c *Chunk) {
	w.chunks.Delete(c.pos)
	c.Destroy()
}

// Returns the ground block from the provided coordinate.
// i.e. the y for a given x,z.
func (w *World) Ground(x, z float32) *Block {
	for y := chunkHeight - 1; y >= 0; y-- {
		b := w.Block(mgl32.Vec3{x, float32(y), z})
		if b != nil && b.active {
			return b
		}
	}
	return nil
}

// Returns the nearby chunks.
// Despaws chunks that are far away.
func (w *World) NearChunks(p mgl32.Vec3) []*Chunk {
	o := make([]*Chunk, 0)
	for _, c := range w.chunks.All() {
		chunkCenter := c.pos.Add(mgl32.Vec3{chunkWidth / 2, chunkHeight / 2, chunkWidth / 2})
		diff := p.Sub(chunkCenter)
		diffl := diff.Len()
		if diffl <= visibleRadius {
			o = append(o, c)
		} else if diffl > destroyRadius {
			w.DespawnChunk(c)
		}
	}

	return o
}

// Ensures that the radius around this center is spawned.
// TODO: find a way to speed up
func (w *World) SpawnRadius(center mgl32.Vec3) {
	r := float32(visibleRadius)
	arc := chunkWidth / float32(2.0)
	theta := arc / r

	// number of itterations is circ/arc
	iterations := int((2 * math.Pi * r) / arc)

	v := mgl32.Vec2{r, 0}
	for i := 0; i < iterations; i++ {
		// simply call block to trigger a spawn if chunk doesnt exist
		p := center.Add(mgl32.Vec3{v.X(), 0, v.Y()})
		w.Block(p)

		// rotate vector
		m := mgl32.Rotate2D(theta)
		v = m.Mul2x1(v)
	}
}

// Returns the block at the given position.
// This takes any position in the world, including non-round postions.
// Will spawn chunk if it doesnt exist yet.
func (w *World) Block(pos mgl32.Vec3) *Block {
	floor := func(v float32) int {
		return int(math.Floor(float64(v)))
	}
	x, y, z := floor(pos.X()), floor(pos.Y()), floor(pos.Z())

	// remainder will be the offset inside chunk
	xoffset := x % chunkWidth
	yoffset := y % chunkHeight
	zoffset := z % chunkWidth

	// if the offsets are negative we flip
	// because chunk origins are at the lower end corners
	if xoffset < 0 {
		// offset = chunkSize - (-offset)
		xoffset = chunkWidth + xoffset
	}
	if yoffset < 0 {
		yoffset = chunkHeight + yoffset
	}
	if zoffset < 0 {
		zoffset = chunkWidth + zoffset
	}

	// get the chunk origin position
	startX := x - xoffset
	startY := y - yoffset
	startZ := z - zoffset

	chunkPos := mgl32.Vec3{float32(startX), float32(startY), float32(startZ)}
	chunk := w.chunks.Get(chunkPos)
	if chunk == nil {
		chunk = w.SpawnChunk(chunkPos)
	}

	block := chunk.blocks[xoffset][yoffset][zoffset]
	return block
}

// Spawns tress on a chunk.
func (w *World) SpawnTrees(chunk *Chunk) {
	biome := w.generator.Biome(mgl32.Vec2{chunk.pos.X(), chunk.pos.Z()})
	trunkHeight := float32(7.0)
	trees := w.generator.TreeDistribution(mgl32.Vec2{chunk.pos.X(), chunk.pos.Z()})
	isSmallGenerator := rand.New(rand.NewSource(w.generator.noise.seed + int64(chunk.pos[0])))
	for x, dist := range trees {
		for z, prob := range dist {
			if prob <= 0.65 {
				continue
			}

			b := w.Ground(chunk.pos.X()+float32(x), chunk.pos.Z()+float32(z))
			if b == nil {
				continue
			}

			base := b.WorldPos()

			// trunk
			for i := 1; i < int(trunkHeight); i++ {
				block := w.Block(base.Add(mgl32.Vec3{0, float32(i), 0}))
				block.active = true
				if biome < 0.4 {
					block.blockType = "cactus"
				} else {
					if int(prob*100)%2 == 0 {
						block.blockType = "dark-wood"
					} else if int(prob*1000)%2 == 0 {
						block.blockType = "white-wood"
					} else {
						block.blockType = "wood"
					}
				}
			}

			// dont draw leaves
			if biome <= 0.4 {
				continue
			}

			width := float32(5.0)
			leavesHeight := float32(3.0)
			small := isSmallGenerator.Float64() > 0.5
			if small {
				width = float32(3.0)
			}
			corner := base.Add(mgl32.Vec3{-float32(width) / 2, trunkHeight - 2, -float32(width) / 2})
			for y := 0; y < int(leavesHeight); y++ {
				layerWidth := width - float32(y)*2
				start := (width - layerWidth) / 2

				for x := 0; x < int(layerWidth); x++ {
					for z := 0; z < int(layerWidth); z++ {
						pos := corner.Add(mgl32.Vec3{start + float32(x), float32(y), start + float32(z)})
						block := w.Block(pos)

						if x == int(layerWidth)/2 && z == int(layerWidth)/2 {
							if y < int(leavesHeight)-1 {
								block.active = true
								block.blockType = "wood"
							}

							if !small {
								continue
							}
						}

						block.active = true
						block.blockType = "leaves"
					}
				}
			}
		}
	}
}
