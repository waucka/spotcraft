#!/usr/bin/env python3

import yaml
import json
import sys

with open(sys.argv[1], 'r') as f:
    print(json.dumps(yaml.load(f)))
