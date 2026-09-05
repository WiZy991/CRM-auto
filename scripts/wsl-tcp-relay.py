#!/usr/bin/env python3
"""Проброс stdin/stdout к TCP-порту внутри WSL."""
import os
import socket
import sys
import threading

port = int(sys.argv[1])
sock = socket.create_connection(("127.0.0.1", port), timeout=5)
sock.settimeout(None)


def stdin_to_sock() -> None:
    try:
        while True:
            data = os.read(0, 65536)
            if not data:
                break
            sock.sendall(data)
    finally:
        try:
            sock.shutdown(socket.SHUT_WR)
        except OSError:
            pass


def sock_to_stdout() -> None:
    try:
        while True:
            data = sock.recv(65536)
            if not data:
                break
            os.write(1, data)
    except OSError:
        pass


thread = threading.Thread(target=stdin_to_sock, daemon=True)
thread.start()
sock_to_stdout()
thread.join(timeout=1)
