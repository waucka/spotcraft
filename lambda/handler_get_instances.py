import os
import json
import boto3

from .common import get_tag, get_minecraft_instances

def handler(event, context):
    ec2 = boto3.client('ec2')

    if 'EC2_TAG' in os.environ:
        tag_name = os.environ['EC2_TAG']
    else:
        tag_name = 'MinecraftServer'

    if 'VPC_ID' in os.environ:
        vpc_id = os.environ['VPC_ID']
    else:
        vpc_id = None

    instances = get_minecraft_instances(ec2, tag_name, vpc_id)

    response_payload = []
    for inst in instances:
        response_payload.append({
            'instance_id': inst['InstanceId'],
            'server_name': get_tag(inst, tag_name),
        })

    return {
        'statusCode': 200,
        'body': json.dumps(response_payload),
    }
