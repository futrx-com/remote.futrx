import readline from 'node:readline';
const send = value => process.stdout.write(JSON.stringify(value)+'\n');
for await (const line of readline.createInterface({input:process.stdin})) {
 const request=JSON.parse(line);if(request.id===undefined)continue;
 const result=request.method==='initialize'?{protocolVersion:'2024-11-05',capabilities:{tools:{}},serverInfo:{name:'fixture',version:'1'}}:request.method==='tools/list'?{tools:[{name:'browser_snapshot',description:'Fixture browser snapshot',inputSchema:{type:'object',properties:{}}}]}:request.method==='tools/call'?{content:[{type:'text',text:'BROWSER_FIXTURE_OK'}]}:{};
 send({jsonrpc:'2.0',id:request.id,result});
}
