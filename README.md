# kubequery-postgres
centralized kubequery postgres database.

## Architecture

Create a centralized 'loader' that polls a collection of kubernetes clusters using kubequery and store all results in a centralized
postgres database using the same schema as kubequery. 


## Features

kubequery is normally installed on specific clusters and can be used to query the local kubernetes resources using SQL.

This is a different approach where kubequery is installed on only one cluster, polls all other clusters using kubequery remote access
and save the data in a postgres database with the following benefits:

- Allows to store kube resources from hundreds of clusters in a single postgres database.
- Allows to make almost real-time queries against hundreds of clusters since all the data is in the same postgres database.
- Index can be added to the PG database for improved access time. 
- Polling the clusters and saving the results in the database is fast and can be done every 20 minutes if needed. Clusters can be polled in parallel for improved speed. 
- On large clusters, SQL joins can be really slow against the sqlite database used by kubequery. The postgres tables are not virtual and can include indexes tosolve this problem.
- Making queries against multiple clusters is easy since all the data is in a single database.

## Kubequery changes

For this application, kubequery had to be modified to support remote cluster access. Currently, kubequery assumes that it is running
inside the k8s cluster being probed. This change allows kubequery to connect to a remote cluster  and extract resource data.

Example kubequery call using parameters:

```
#!/bin/sh
# runquery <table> <token> <cluster_addr> <cluster> <uuid> <ix>

TABLE=$1
export KUBEQUERY_TOKEN=$2
export KUBEQUERY_ADDR=$3
export CLUSTER_NAME=$4
export CLUSTER_UID=$5
export CLUSTER_IX=$6

echo "select * from $TABLE;" | /opt/uptycs/bin/basequery  --flagfile=/opt/uptycs/etc/kubequery.flags  --config_path=/opt/uptycs/etc/kubequery.conf --extensions_socket=/opt/uptycs/var/kubequeryi.em$TABLE.$CLUSTER_IX  --extensions_autoload=/opt/uptycs/etc/autoload.exts  --extensions_require=kubequery  --extension_event_tables=kubernetes_events  --disable_database  --json  --disable_events=false  -S > /tmp/${CLUSTER_NAME}-$TABLE.json

```

## Additional fields

The loader has access to a dictionary that describes the changes that are needed to converts from  sqlite to postgres. 
This dictionary also allows to add extra fields to the postres tables. These fields are application-specific and can be customized.

For example, in Comcast-RDEI, the clusters are organized in 'regions' and the namespaces are organized in 'teams'. It is possible to get these new columns using complex sql joins but for performance and ease of use, the tables can be denormalized and these columns can be added after their parent. 

Example:
``` 
# Original kubequery schema:
 cluster_name TEXT,
 cluster_id  TEXT,

# Postgres schema:
  cluster_name varchar(256),
  region varchar(256),         <<< this field in added in the postgres schema and included by the loader.
  cluster_id varchar(256)
```


## Example queries

Note: The strings have been obfuscated.

```
# PODS BY REGIONS
> select region, count(distinct cluster_name) cluster_cnt, count(distinct namespace) ns_count , count(*) pod_cnt from kubernetes_pods group by region;
        region         | cluster_cnt | ns_count | pod_cnt 
-----------------------+-------------+----------+---------
 national1             |           4 |      343 |    9636
 national2             |          25 |       43 |   30324
 national3             |           4 |       29 |    9988
 regional1             |           8 |       49 |    8342
 regional2             |           8 |       50 |    6845
 regional4             |           4 |       16 |    2943
 regional5             |           6 |       33 |    3062
 regional6             |           6 |       38 |    4489
 regional7             |           8 |       33 |    7748
 regional8             |           3 |       42 |    1650
 regional9             |           3 |       36 |   20784

(11 rows)

Time: 314.390 ms


# MOST POPULAR IMAGES
> select c.image, count(*) pod_cnt from kubernetes_pod_containers c group by c.image order by count(*) desc limit 10;
                            image                             | pod_cnt
--------------------------------------------------------------+--------
 hub.obfusca.net/xxxxx-packager/system/system:3.5.1-1         | 18080
 hub.obfusca.net/xxxxx-arch/varnish-image:7.0.2-dev1          | 11376
 hub.obfusca.net/xxxxx-packager/system/system:3.3.2-1         | 10820
 hub.obfusca.net/library/telegraf:1.13.4                      |  5434
 registry.xxxxxxx.net/anchorfree/twemproxy:latest             |  5384
 hub.obfusca.net/rio/services/rccs-decoder:1.1.13_959         |  4217
 hub.obfusca.net/k8s-eng/xxxxx/rdei/k8s-dns-node-cache:1.21.1 |  3727
 hub.obfusca.net/xxxxx/rdei/node-exporter:v1.3.1              |  3382
 hub.obfusca.net/k8s-eng/xxxxx/rdei/sumatra:0.42.07           |  3315
 hub.obfusca.net/rio/services/bmw:1.9.0_1473                  |  3299
(10 rows)

Time: 230.233 ms
 
```


