
image: FORCE
	docker build -t kubequery-postgres:2 .
	docker push kubequery-postgres:2



loader: FORCE
	go build -o loader ./cmd/...

testrun: FORCE
	./testrun

FORCE:	;
