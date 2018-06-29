import os
import json
import boto3

def handler(event, context):
    ec2 = boto3.client('ec2')
    s3 = boto3.client('s3')

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

    instances = get_minecraft_instances(ec2, tag_name, vpc_id)

    instance_id = event['pathParameters']['instance_id']
    found = False
    for inst in instances:
        if inst['InstanceId'] == instance_id:
            found = True
            break
    if !found:
        return {
            'statusCode': 404,
            'body': json.dumps({'message': "No such instance: {instance_id}".format(instance_id=instance_id)}),
        }

    if 'Terminate' in event['headers'] and event['headers']['Terminate'] == 'y':
        terminate = True
    else:
        terminate = False

    if terminate:
        ec2.terminate_instances(InstanceIds=[instance_id])
    else:
        ec2.stop_instances(InstanceIds=[instance_id])

    return {
        'statusCode': 200,
        'body': json.dumps(servers),
    }
