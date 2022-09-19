FROM uptycs/kubequery:kubequery.1.1.1.patch


COPY database/schema_sqlite.sql /schema.sql
COPY loader /loader
COPY runquery /runquery
COPY loop /loop

ENTRYPOINT ["/loop"]

