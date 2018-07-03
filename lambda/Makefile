FUNCTION_PY := $(shell ls handler*.py)
FUNCTION_ZIP := $(subst handler_,,$(FUNCTION_PY:.py=.zip))

all: $(FUNCTION_ZIP)

%.zip: handler_%.py common.py
	echo zip $@ $^

clean:
	rm -f $(FUNCTION_ZIP)

.PHONY: all clean 
