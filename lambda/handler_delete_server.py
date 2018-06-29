import os
import json
import boto3

from .common import get_tag, get_minecraft_instances

def handler(event, context):
    s3 = boto3.client('s3')
    ec2 = boto3.client('ec2')

    if 'BUCKET_NAME' in os.environ:
        bucket_name = os.environ['BUCKET_NAME']
    else:
        return {
            'statusCode': 500,
            'body': json.dumps({
                'message': 'BUCKET_NAME is not defined!'
            }),
        }

    if 'EC2_TAG' in os.environ:
        tag_name = os.environ['EC2_TAG']
    else:
        tag_name = 'MinecraftServer'

    if 'VPC_ID' in os.environ:
        vpc_id = os.environ['VPC_ID']
    else:
        vpc_id = None

    server_name = event['pathParameters']['serverName']
    instances = get_minecraft_instances(ec2, tag_name, vpc_id)
    for inst in instances:
        if get_tag(inst, tag_name) == server_name:
            return {
                'statusCode': 400,
                'body': json.dumps({
                    'message': "Server {server_name} still has a running instance ({instance_id})".format(server_name=server_name, instance_id=inst['InstanceId']),
                }),
            }

    s3_get_response = s3.get_object(
        Bucket=bucket_name,
        Key="servers/{server_name}/config.json".format(server_name=server_name),
    )
    server_config = json.loads(s3_get_response['Body'])
    ec2.delete_volume(VolumeId=server_config['world_volume_id'])
    ec2.release_address(AllocationId=server_config['eip_id'])

    return {
        'statusCode': 200,
        'body': json.dumps(servers),
    }
