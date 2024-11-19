# CONTIRUBUTING

The Udash project is a web application designed to better understand the state of Git repository.

It's composed of:
* A Golang HTTP service (This repository) 
* A Vue.js Frontend [updatecli/udash-front](https://github.com/updatecli/udash-front)
* Updatecli as the agent

The following instruction is the best way to start Udash within a dev environment.

## 1. Database

Udash relies on a postgresql database to store its data.
We can spin up a local one running

* `make db.start`

Then we can test sql command running

* `PGPASSWORD=password psql --username=udash --file test.sql  postgres://localhost:5432/udash `

## 2. Server

The server fetches its configuration from one of the following files (order matter):

1. "config.yaml"
2. "$HOME/.udash/config.yaml"
3. "/etc/udash/config.yaml"

```config.yaml
server:
  auth:
    ## Disable authentication
    mode: "none"
    ## If dryrun is set to true, then only GET request are allowed
    dryrun: false

database:
  uri: "postgres://udash:password@localhost:5432/udash?sslmode=disable"
```

* `make server.start`

### Endpoints

The file `pkg/server/main.go` contains the following endpoint:

* `/api/ping` [GET]
* `/api/about`[GET]
* `/api/pipeline/scms`[GET]
* `/api/pipeline/reports`[GET][POST]
* `/api/pipeline/reports/:id`[GET][PUT][DELETE]

## 3. Frontend

Information to start the frontend is available on [github.com/updatecli/udash-front](https://github.com/updatecli/udash-front)


## 4. Agent

The Updatecli command is used to provide all the data to Udash.
Please be aware that Udash is currently an experimental feature
within Updatecli and must be enabled with the flag `--experimental`
More information on `updatecli udash --help --experimental`