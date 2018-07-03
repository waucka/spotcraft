FUNCTION_ZIP := $(shell ls ../lambda/*.zip)

all: functions
	cp $(FUNCTION_ZIP) functions/
	../convert_swagger.py ../api.yaml > ./api.json
	cp ../spotcraft.template ./

functions:
	mkdir -p functions

clean:
	rm -rf functions

.PHONY: all clean functions api.json spotcraft.template
