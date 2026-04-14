APP_NAME := goat
DEST := /opt/$(APP_NAME)
SERVICE_PATH := /etc/systemd/system/$(APP_NAME).service

.PHONY: all build install uninstall

all: build

build:
	go build -o $(APP_NAME) .

install: build
	sudo mkdir -p $(DEST)
	sudo install -m 755 $(APP_NAME) $(DEST)/$(APP_NAME)
	echo "[Unit]\n\
Description=$(APP_NAME) Service\n\
After=network.target\n\n\
[Service]\n\
Type=simple\n\
ExecStart=$(DEST)/$(APP_NAME)\n\
Restart=on-failure\n\
WorkingDirectory=$(DEST)\n\
DynamicUser=yes\n\
ProtectSystem=strict\n\
StateDirectory=$(APP_NAME)\n\n\
[Install]\n\
WantedBy=multi-user.target" | sudo tee $(SERVICE_PATH) > /dev/null
	sudo systemctl daemon-reload
	sudo systemctl enable $(APP_NAME)
	sudo systemctl restart $(APP_NAME)

uninstall:
	sudo systemctl stop $(APP_NAME) || true
	sudo systemctl disable $(APP_NAME) || true
	sudo rm -f $(SERVICE_PATH)
	sudo rm -rf $(DEST)
	sudo systemctl daemon-reload
