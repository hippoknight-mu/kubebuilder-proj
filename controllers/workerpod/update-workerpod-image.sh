set -ex
GOOS=linux go build -o ./app .

docker build -t cosmictestacr.azurecr.io/cdt-worker:latest .
docker push cosmictestacr.azurecr.io/cdt-worker:latest

