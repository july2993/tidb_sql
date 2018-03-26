## tidb_sql.go

use [pcap](https://godoc.org/github.com/google/gopacket/pcap) to read packets off the wire about tidb-server(or mysql), and print the sql which client send to server in stdout (some log will be print in stderr).

for the [prepared-staytements](https://dev.mysql.com/doc/internals/en/prepared-statements.html) it will print it in the [text-protocol](https://dev.mysql.com/doc/internals/en/text-protocol.html) way.



## test.go

create a test table *test_sql.test* and insert some date to it on 127.0.0.1:4000 for test



## how to use it

after build tidb_sql(go build tidb_sql.go), run it like (you can specify the Interface and the port of tidb-server:

```
➜  tidb_sql git:(master) ✗ sudo ./tidb_sql -i lo -port 4000 2>&/dev/null
```

in another terminal run test.go

```
➜  tidb_sql git:(master) ✗ go run test.go
➜  tidb_sql git:(master) ✗
```



you will see the output sql of tidb_sql:

```
➜  tidb_sql git:(master) ✗ sudo ./tidb_sql -i lo -port 4000 2>&/dev/null
drop database if exists test_sql;
create database test_sql;
create table test_sql.test(a int, b varchar(255));
# exec prepare stmt:  insert into test_sql.test values( ?, ? );
# binary exec a prepare stmt rewrite it like:
prepare stmt1 from 'insert into test_sql.test values( ?, ? )';
set @p0 = 1024;
set @p1 = '1024';
execute stmt1  using @p0, @p1;
drop prepare stmt1;

# exec prepare stmt:  insert into test_sql.test values( ?, ? );
# binary exec a prepare stmt rewrite it like:
prepare stmt1 from 'insert into test_sql.test values( ?, ? )';
set @p0 = NULL;
set @p1 = NULL;
execute stmt1  using @p0, @p1;
drop prepare stmt1;

# exec prepare stmt:  insert into test_sql.test values( ?, ? );
# binary exec a prepare stmt rewrite it like:
prepare stmt1 from 'insert into test_sql.test values( ?, ? )';
set @p0 = 2;
set @p1 = '2';
execute stmt1  using @p0, @p1;
drop prepare stmt1;

# exec prepare stmt:  insert into test_sql.test values( ?, ? );
# binary exec a prepare stmt rewrite it like:
prepare stmt1 from 'insert into test_sql.test values( ?, ? )';
set @p0 = 1;
set @p1 = '1';
execute stmt1  using @p0, @p1;
drop prepare stmt1;

# exec prepare stmt:  insert into test_sql.test values( ?, ? );
# binary exec a prepare stmt rewrite it like:
prepare stmt1 from 'insert into test_sql.test values( ?, ? )';
set @p0 = 4;
set @p1 = '4';
execute stmt1  using @p0, @p1;
drop prepare stmt1;

# exec prepare stmt:  insert into test_sql.test values( ?, ? );
# binary exec a prepare stmt rewrite it like:
prepare stmt1 from 'insert into test_sql.test values( ?, ? )';
set @p0 = 0;
set @p1 = '0';
execute stmt1  using @p0, @p1;
drop prepare stmt1;

# exec prepare stmt:  insert into test_sql.test values( ?, ? );
# binary exec a prepare stmt rewrite it like:
prepare stmt1 from 'insert into test_sql.test values( ?, ? )';
set @p0 = 3;
set @p1 = '3';
execute stmt1  using @p0, @p1;
drop prepare stmt1;

insert into test_sql.test values( 1, '1' );
insert into test_sql.test values( 2, '2' );
insert into test_sql.test values( 4, '4' );
insert into test_sql.test values( 0, '0' );
insert into test_sql.test values( 3, '3' );
```



you can redirect the output sql of *tidb_sql* to another mysql to replay the sql:

```
➜  tidb_sql git:(master) ✗ sudo ./tidb_sql -i lo -port 4000 2>&/dev/null | mysql -uroot -phello
mysql: [Warning] Using a password on the command line interface can be insecure.
```

after run test.go to create table *tidb_sql.test* and insert some data:



check tidb and mysql:

tidb:

```
➜  tidb_sql git:(master) ✗ mysql -h 127.0.0.1 -P 4000 -u root
Welcome to the MySQL monitor.  Commands end with ; or \g.
Your MySQL connection id is 449
Server version: 5.7.1-TiDB-v1.1.0-alpha-357-gb1e1a26 MySQL Community Server (Apache License 2.0)

Copyright (c) 2000, 2018, Oracle and/or its affiliates. All rights reserved.

Oracle is a registered trademark of Oracle Corporation and/or its
affiliates. Other names may be trademarks of their respective
owners.

Type 'help;' or '\h' for help. Type '\c' to clear the current input statement.

mysql> select * from test_sql.test order by a;
+------+------+
| a    | b    |
+------+------+
| NULL | NULL |
|    0 | 0    |
|    0 | 0    |
|    1 | 1    |
|    1 | 1    |
|    2 | 2    |
|    2 | 2    |
|    3 | 3    |
|    3 | 3    |
|    4 | 4    |
|    4 | 4    |
| 1024 | 1024 |
+------+------+
12 rows in set (0.00 sec)
```



mysql:

```shell
➜  tidb_sql git:(master) ✗ mysql -uroot -phello
mysql: [Warning] Using a password on the command line interface can be insecure.
Welcome to the MySQL monitor.  Commands end with ; or \g.
Your MySQL connection id is 16
Server version: 5.7.21-0ubuntu0.16.04.1-log (Ubuntu)

Copyright (c) 2000, 2018, Oracle and/or its affiliates. All rights reserved.

Oracle is a registered trademark of Oracle Corporation and/or its
affiliates. Other names may be trademarks of their respective
owners.

Type 'help;' or '\h' for help. Type '\c' to clear the current input statement.

mysql> select * from test_sql.test order by a;
+------+------+
| a    | b    |
+------+------+
| NULL | NULL |
|    0 | 0    |
|    0 | 0    |
|    1 | 1    |
|    1 | 1    |
|    2 | 2    |
|    2 | 2    |
|    3 | 3    |
|    3 | 3    |
|    4 | 4    |
|    4 | 4    |
| 1024 | 1024 |
+------+------+
12 rows in set (0.01 sec)
```

but the db may not be consistent in this way always, because:

- the output order of sql may not like the order of the origin server execute  when there are multiple conccurent connection.
- some sql like **update test_sql.test set b = now();** will not get the same result when just replay it.
