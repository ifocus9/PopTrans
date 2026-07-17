import os


DEFAULT_SERVER_PORT = 8989
MIN_SERVER_PORT = 1024
MAX_SERVER_PORT = 65535
SERVER_PORT_ENV = "TRANSLATE_SERVER_PORT"


def server_port(environ=None):
    source = os.environ if environ is None else environ
    try:
        port = int(source.get(SERVER_PORT_ENV, DEFAULT_SERVER_PORT))
    except (TypeError, ValueError):
        return DEFAULT_SERVER_PORT

    if MIN_SERVER_PORT <= port <= MAX_SERVER_PORT:
        return port
    return DEFAULT_SERVER_PORT
