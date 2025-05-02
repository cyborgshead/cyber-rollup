# This Kurtosis package spins up a Cyber rollup that connects to a DA node
#
# NOTE: currently this is only connecting to a local DA node

da_node = import_module("github.com/rollkit/local-da/main.star@v0.3.0")

def run(plan):
    ##########
    # DA
    ##########

    da_address = da_node.run(
        plan,
    )
    plan.print("connecting to da layer via {0}".format(da_address))

    #################
    # Cyber Rollup
    #################
    plan.print("Adding Cyber service")
    rpc_port_number = 26657
    grpc_port_number = 9090
    p2p_port_number = 26656
    api_port_number = 1317
    cyber_start_cmd = [
        "cyber",
        "start",
        "--rollkit.aggregator",
        "--rollkit.da_address {0}".format(da_address),
        "--rollkit.block_time=3s",
        "--rpc.laddr tcp://0.0.0.0:{0}".format(rpc_port_number),
        "--grpc.address 0.0.0.0:{0}".format(grpc_port_number),
        "--p2p.laddr 0.0.0.0:{0}".format(p2p_port_number),
        "--api.address 0.0.0.0:{0}".format(api_port_number),
        "--minimum-gas-prices='0.15ustake'",
    ]
    cyber_ports = {
        "rpc-laddr": defaultPortSpec(rpc_port_number),
        "grpc-addr": defaultPortSpec(grpc_port_number),
        "p2p-laddr": defaultPortSpec(p2p_port_number),
        "api-addr": defaultPortSpec(api_port_number),
    }
    cyber = plan.add_service(
        name="cyber",
        config=ServiceConfig(
            # image="ghcr.io/cyborgshead/cyber-rollup:41a63d0",
            # Use ImageBuildSpec when testing changes to Dockerfile
            image = ImageBuildSpec(
                image_name="cyber-rollup",
                build_context_dir=".",
            ),
            cmd=["/bin/sh", "-c", " ".join(cyber_start_cmd)],
            ports=cyber_ports,
            public_ports=cyber_ports,
            ready_conditions=ReadyCondition(
                recipe=ExecRecipe(
                    command=[
                        "cyber",
                        "status",
                        "-n",
                        "tcp://127.0.0.1:{0}".format(rpc_port_number),
                    ],
                    extract={
                        "output": "fromjson | .node_info.network",
                    },
                ),
                field="extract.output",
                assertion="==",
                target_value="cyber42-1",
                interval="1s",
                timeout="10s",
            ),
        ),
    )

    cyber_address = "http://{0}:{1}".format(
        cyber.ip_address, cyber.ports["rpc-laddr"].number
    )
    plan.print("Cyber service is available at {0}".format(cyber_address))


def defaultPortSpec(port_number):
    return PortSpec(
        number=port_number,
        transport_protocol="TCP",
        application_protocol="http",
    )
