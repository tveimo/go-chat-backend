FROM golang:1.24.3-alpine AS build-env

# Build env

RUN apk add build-base git

ENV GOPATH /workspace/
ENV GOPROXY https://proxy.golang.org

ENV GOBIN /workspace/areo/go-chat-backend/bin

ADD ./src/go-chat-backend $GOPATH/areo/go-chat-backend
WORKDIR $GOPATH/areo/go-chat-backend

RUN make clean
RUN make build

RUN ls -alF /workspace/areo/go-chat-backend/emailtemplates

RUN apk del build-base git

# Runtime env

FROM alpine
WORKDIR /api
RUN apk add ca-certificates

COPY --from=build-env /workspace/areo/go-chat-backend/bin /api
COPY --from=build-env /workspace/areo/go-chat-backend/emailtemplates /api/emailtemplates

RUN ls -alF /api
RUN ls -alF /api/emailtemplates
ENTRYPOINT ["/api/go-chat-backend"]
