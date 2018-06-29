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

    iam_instance_profile = os.environ['IAM_INSTANCE_PROFILE_ARN']
    log_group = os.environ['LOG_GROUP']

    request = json.loads(event['body'])
    server_name = request['server_name']
    duration = request['duration']
    key_name = request['key_name']
    if duration > 0:
        spot_options = {
            'SpotInstanceType': 'one-time',
            'BlockDurationMinutes': duration,
            'InstanceInterruptionBehavior': 'terminate',
        }
    else:
        spot_options = {
            'SpotInstanceType': 'persistent',
            'InstanceInterruptionBehavior': 'terminate',
        }

    if key_name == '':
        key_name = None

    s3_get_response = s3.get_object(
        Bucket=bucket_name,
        Key="servers/{server_name}/config.json".format(server_name=server_name),
    )
    server_config = json.loads(s3_get_response['Body'])

    ec2_response = ec2.run_instances(
        BlockDeviceMappings=[
            {
                'DeviceName': '/dev/sda1',
                'Ebs': {
                    'DeleteOnTermination': True,
                    'VolumeType': 'gp2',
                    'VolumeSize': 20,
                },
            },
        ],
        ImageId=server_config['ami_id'],
        KeyName=key_name,
        InstanceType=server_config['ec2_type'],
        InstanceMarketOptions={
            'MarketType': 'spot',
            'SpotOptions': spot_options,
        },
        InstanceInitiatedShutdownBehavior='terminate',
        IamInstanceProfile={
            'Arn': iam_instance_profile,
        },
        UserData=json.dumps({
	    'elastic_ip': server_config['eip_id'],
	    'ebs_volume': server_config['world_volume_id'],
	    'log_group': log_group,
	    'log_stream': server_name,
        }),
        TagSpecifications=[
            {
                'Tags': [
                    {
                        'Key': tag_name,
                        'Value': server_name,
                    },
                ],
            },
        ],
    )

    return {
        'statusCode': 200,
        'body': json.dumps(servers),
    }
