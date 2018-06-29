def get_tag(inst, tag_name):
    for tag in inst['Tags']:
        if tag['Key'] == tag_name:
            return tag['Value']
    return None

def get_minecraft_instances(ec2, tag_name, vpc_id):
    filters = [
        {
            'Name': 'tag-key',
            'Values': [
                tag_name,
            ],
        },
    ]
    if vpc_id is not None:
        filters.append({
            'Name': 'vpc-id',
            'Values': [
                vpc_id,
            ],
        })

    instances = []
    nextToken = None
    while True:
        response = ec2.describe_instances(Filters=filters, MaxResults=10, NextToken=nextToken)
        for resv in response['Reservations']:
            instances.append(resv['Instances'])
        if 'NextToken' not in response or response['NextToken'] is None:
            break
        nextToken = response['NextToken']

    return instances
