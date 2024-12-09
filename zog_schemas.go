package main

import z "github.com/Oudwins/zog"

var notificationRequestSchema = z.Slice(z.Struct(z.Schema{
	"user":        z.String().Required(z.Message("users array is required")).Min(1, z.Message("user cannot be empty")),
	"title":       z.String().Required(z.Message("title is required")).Min(1, z.Message("title cannot be empty")),
	"description": z.String().Required(z.Message("description is required")).Min(1, z.Message("description cannot be empty")),
	"link":        z.String().Required(z.Message("link is required")).Min(1, z.Message("link cannot be empty")),
}))

var broadcastRequestSchema = z.Struct(z.Schema{
	"title":       z.String().Required(z.Message("title is required")).Min(1, z.Message("title cannot be empty")),
	"description": z.String().Required(z.Message("description is required")).Min(1, z.Message("description cannot be empty")),
	"link":        z.String().Required(z.Message("link is required")).Min(1, z.Message("link cannot be empty")),
})

var pushSubscriptionSchema = z.Struct(z.Schema{
	"endpoint": z.String().Required(z.Message("endpoint URL is required")),
	"keys": z.Struct(z.Schema{
		"auth":   z.String().Required(z.Message("auth key is required")),
		"p256dh": z.String().Required(z.Message("p256dh key is required")),
	}),
})
