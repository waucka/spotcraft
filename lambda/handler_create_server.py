import os
import json
import boto3

def handler(event, context):
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

    request = json.loads(event['body'])
    server_name = event['pathParameters']['serverName']

    s3_put_response = s3.put_object(
        Bucket=bucket_name,
        Key="servers/{server_name}/config.json".format(server_name=request['server_name']),
        Body=json.dumps(request),
    )

    return {
        'statusCode': 200,
        'body': json.dumps(servers),
    }
