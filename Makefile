SUBDIRS := minemanagerd mounter unmounter find-nvme-device
PACKER := packer

all: $(SUBDIRS)

$(SUBDIRS):
	$(MAKE) -C $@

# If you want to use the defaults, you don't need to create
# vars.json yourself; make will do it for you.
vars.json:
	echo "{}" > vars.json

ami: $(SUBDIRS) packer.json vars.json
	$(PACKER) validate packer.json
	$(PACKER) build -var-file=vars.json packer.json

clean:
	bash ./clean.sh $(SUBDIRS)

.PHONY: all clean ami $(SUBDIRS)
