# everynyan-notification-service

This is the notifications service for [everynyan](https://github.com/shravanasati/everynyan) which is resposible for sending in-app and push notifications to users.

It is currently under development.

### Setup Development Environment

1. Clone the repository.

2. Create a `.env` file.

```
SECRET_KEY=
SALT=
API_KEY=

VAPID_PRIVATE_KEY=
VAPID_PUBLIC_KEY=
```

The first three fields should be exactly same as those setup for the [nextjs website](https://github.com/shravanasati/everynyan). 

The `API_KEY` field corresponds to the `NOTIFICATIONS_API_KEY` env var for the nextjs website.

VAPID credentials can be obtained by running `go build`. The server will panic that these env vars are not set and print a set of them, the VAPID private and public key **in order (private first, public second)**.

Set those and then run the server again.

```
go build
```