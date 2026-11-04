// Package ws holds the WebSocket handling for the realtime service.
package ws

import "github.com/coder/websocket"

// Accept upgrades an HTTP request to a WebSocket connection.
var Accept = websocket.Accept
