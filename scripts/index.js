const Web3 = require('web3');
const web3 = new Web3('http://localhost:8545');

// Error data
const errorData = '0x00f20c5d4807f75147f1a4c6d38b53c3aad02cf3f867d9addb4cd1eb965233f76f3d4d72';

// Decode the revert reason if available
web3.eth.call({
  from: '0xe2148ee53c0755215df69b2616e552154edc584f',
  to: '0x18d19c5d3e685f5be5b9c86e097f0e439285d216',
  data: '0x8f111f3c...'
}, 'latest').then(console.log).catch((err) => {
  console.error('Error:', err);
});
