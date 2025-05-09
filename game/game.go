package game

import (
	"log"

	"github.com/go-gl/gl/v4.1-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)

// Main game.
type Game struct {
	// resources
	window   *Window
	shaders  *ShaderManager
	textures *TextureManager

	// main player
	player *Player

	// light source
	light *Light

	// world spawns and despawns entities
	world *World

	// block the player is currently looking at
	target *TargetBlock

	// crosshair shows a cross on the screen
	crosshair *Crosshair

	// hotbar displays inventory bar
	hotbar *Hotbar

	// provides time delta for game loop
	clock *Clock

	// physics engine for player movements and collisions
	physics *PhysicsEngine
}

// Starts the game.
func Start() {
	log.Println("Starting game...")
	g := Game{}
	g.Init()
	g.Run()
}

// Initializes the app. Executes before the game loop.
func (g *Game) Init() {
	// glfw window
	g.window = newWindow()

	gl.Enable(gl.DEPTH_TEST)
	g.light = newLight()
	g.light.SetLevel(1.0)

	// day and night (uncomment to togggle along with `HandleChange()` in the game loop)
	// g.light.StartDay(time.Second * 10)

	// init resource managers and create resources
	g.shaders = newShaderManager("./shaders")
	g.textures = newTextureManager("./assets")
	atlas := newTextureAtlas(g.textures.CreateTexture("atlas.png"))

	g.player = newPlayer()
	g.physics = newPhysicsEngine()
	g.physics.Register(g.player.body)

	g.world = newWorld(g.shaders.Program("chunk"), atlas)
	g.world.Init()
	g.clock = newClock()

	g.SetLookHandler()
	g.SetMouseClickHandler()

	g.crosshair = newCrosshair(g.shaders.Program("crosshair"))
	g.crosshair.Init()

	g.hotbar = newHotbar(g.shaders.Program("hotbar"), atlas, g.player.camera)
	g.hotbar.Init()
}

// Runs the game loop.
func (g *Game) Run() {
	defer g.window.Terminate()
	g.clock.Start()

	for !g.window.ShouldClose() && !g.window.IsPressed(glfw.KeyQ) {
		// clear buffers
		gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)

		// movement
		g.HandleMove()
		g.HandleJump()
		g.HanldleFly()

		// interactions
		g.LookBlock()
		g.HandleInventorySelect()

		// world
		g.world.SpawnRadius(g.player.body.position)
		g.world.ProcessTasks()

		// day/night (uncomment to toggle)
		// g.light.HandleChange()

		delta := g.clock.Delta()
		g.physics.Tick(delta)

		// drawing
		g.crosshair.Draw()
		g.hotbar.Draw()

		for _, c := range g.world.NearChunks(g.player.body.position) {
			// cull chunks that are not in view
			if !g.player.Sees(c) {
				continue
			}

			// if a block is being looked at in this chunk
			var target *TargetBlock
			if g.target != nil && g.target.block.chunk == c {
				target = g.target
			}

			c.Draw(target, g.player.camera, g.light)
		}

		// window maintenance
		g.window.SwapBuffers()
		glfw.PollEvents()
	}
}
