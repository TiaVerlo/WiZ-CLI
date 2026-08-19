#!/bin/bash
# -*- coding: utf-8 -*-

cd "$(dirname "${BASH_SOURCE[0]}")" || exit 1
wiz_cli="./wiz_cli.py"

chmod +x $wiz_cli
python3 $wiz_cli
