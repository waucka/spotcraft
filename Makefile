SUBDIRS := minemanagerd mounter unmounter find-nvme-device cli lambda static-assets
PACKER := packer

all: $(SUBDIRS)

static-assets: lambda api.yaml spotcraft.template
	$(MAKE) -C $@

minemanagerd:
	$(MAKE) -C $@

mounter:
	$(MAKE) -C $@

unmounter:
	$(MAKE) -C $@

find-nvme-device:
	$(MAKE) -C $@

lambda:
	$(MAKE) -C $@

cli: static-assets
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

.PHONY: all clean ami static-assets $(SUBDIRS)
