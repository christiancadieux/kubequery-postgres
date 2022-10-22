FROM uptycs/kubequery:kubequery.1.1.1-remote


COPY database/schema_sqlite.sql /schema.sql
COPY loader /loader
COPY runquery /runquery
COPY loop /loop
COPY dictionary.json /dictionary.json

ENTRYPOINT ["/loop"]

