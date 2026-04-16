MODULE   := github.com/turinglabs/ambox
BINARY   := ambox
PROJECT  := iconic-elevator-394020
REGION   := us-central1
IMAGE    := gcr.io/$(PROJECT)/$(BINARY)

.PHONY: build test test-int docker deploy clean run

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BINARY) ./cmd/ambox

run:
	go run ./cmd/ambox

test:
	go test ./... -v -count=1

test-int:
	go test ./... -v -count=1 -tags=integration

test-race:
	go test ./... -race -count=1

docker:
	docker build -t $(IMAGE):latest .

deploy: docker
	docker push $(IMAGE):latest
	gcloud run deploy $(BINARY) \
		--image $(IMAGE):latest \
		--region $(REGION) \
		--project $(PROJECT) \
		--allow-unauthenticated \
		--port 8080 \
		--memory 256Mi \
		--cpu 1 \
		--min-instances 0 \
		--max-instances 10 \
		--timeout 30

clean:
	rm -f $(BINARY)
