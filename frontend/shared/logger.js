import pino from 'pino';

// TODO configure log output based, e.g. json output
const base = pino({
  level: 'info',
  transport: {
    target: 'pino-pretty',
    options: {
      colorize: true,
      // hideObject: true,
      singleLine: true,
      translateTime: 'SYS:standard',
      ignore: 'pid,hostname',
    },
  },
});

export function moduleLogger(prefix) {
  return base.child({}, {
    msgPrefix: `[${prefix}] `,
  });
}

export default base;
