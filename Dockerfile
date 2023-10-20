FROM golang:alpine AS build
WORKDIR /app
COPY . .
RUN go build -o /app/resource/rollback .

FROM gcr.io/google.com/cloudsdktool/google-cloud-cli:alpine
COPY --from=build /app/resource /bin/
RUN chmod +x /bin/rollback
