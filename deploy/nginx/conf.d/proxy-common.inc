# Общие параметры проксирования на Go-бэкенд.
#
# X-Forwarded-For выставляется шлюзом; приложение доверяет ему только для
# адресов из TRUSTED_PROXY_CIDRS, иначе rate-limit обходился бы подделкой
# заголовка.

proxy_http_version 1.1;
proxy_set_header Connection "";
proxy_set_header Host              $host;
proxy_set_header X-Real-IP         $remote_addr;
proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
proxy_set_header X-Forwarded-Proto $scheme;
proxy_set_header X-Request-Id      $request_id;

proxy_connect_timeout 5s;
proxy_send_timeout    30s;
proxy_read_timeout    30s;

proxy_buffering on;
proxy_buffers 8 16k;
proxy_buffer_size 16k;

# Апстрим сам решает, что кешировать; шлюз не подменяет заголовки приложения.
proxy_pass_header Server;
proxy_intercept_errors off;
