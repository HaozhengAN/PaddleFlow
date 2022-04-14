"""
Copyright (c) 2021 PaddlePaddle Authors. All Rights Reserve.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
"""

#!/usr/bin/env python3
# -*- coding:utf8 -*-

import os
import click
import logging
import sys
import configparser
from paddleflow.client import Client
from paddleflow.cli.output import OutputFormat
from paddleflow.cli.user import user
from paddleflow.cli.queue import queue
from paddleflow.cli.fs import fs
from paddleflow.cli.log import log
from paddleflow.cli.run import run
from paddleflow.cli.pipeline import pipeline
from paddleflow.cli.cluster import cluster
from paddleflow.cli.flavour import flavour

DEFAULT_PADDLEFLOW_PORT = 8080
DEFAULT_FS_HTTP_PORT = 8081
DEFAULT_FS_RPC_PORT = 8082

@click.group()
@click.option('--pf_config', help='the path of default config.')
@click.option('--output', type=click.Choice(list(map(lambda x: x.name, OutputFormat))),
              default=OutputFormat.table.name, show_default=True,
              help='The formatting style for command output.')
@click.pass_context
def cli(ctx, pf_config=None, output=OutputFormat.table.name):
    """paddleflow is the command line interface to paddleflow service.\n
       provide `user`, `queue`, `fs`, `run`, `pipeline`, `cluster`, `flavour` operation commands
    """
    if pf_config:
        config_file = pf_config
    else:
        home_path = os.getenv('HOME')
        config_file = os.path.join(home_path, '.paddleflow/config')
    if not os.access(config_file, os.R_OK):
        click.echo("no config file in %s" % config_file, err=True)
        sys.exit(1)
    config = configparser.RawConfigParser()
    config.read(config_file, encoding='UTF-8')
    if 'user' not in config or 'server' not in config:
        click.echo("no user or server conf in %s" % config_file, err=True)
        sys.exit(1)
    if 'password' not in config['user'] or 'name' not in config['user']:
        click.echo("no name or password conf['user'] in %s" % config_file, err=True)
        sys.exit(1)
    if 'paddleflow_server' not in config['server']:
        click.echo("no paddleflow_server in %s" % config_file, err=True)
        sys.exit(1)
    paddleflow_server = config['server']['paddleflow_server']
    if 'paddleflow_port' in config['server']:
        paddleflow_port = config['server']['paddleflow_port']
    else:
        paddleflow_port = DEFAULT_PADDLEFLOW_PORT
    ctx.obj['client'] = Client(paddleflow_server, config['user']['name'], config['user']['password'], paddleflow_port)
    name = config['user']['name']
    password = config['user']['password']
    ctx.obj['client'].login(name, password)
    ctx.obj['output'] = output


def main():
    """main
    """
    logging.basicConfig(format='%(message)s', level=logging.INFO)
    cli.add_command(user)
    cli.add_command(queue)
    cli.add_command(fs)
    cli.add_command(run)
    cli.add_command(pipeline)
    cli.add_command(cluster)
    cli.add_command(flavour)
    cli.add_command(log)
    try:
        cli(obj={}, auto_envvar_prefix='paddleflow')
    except Exception as e:
        click.echo(str(e), err=True)
        sys.exit(1)