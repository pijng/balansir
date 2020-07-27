import { using } from 'forest';
import { RoutePages } from './routing';

using(document.querySelector('body'), () => {
  RoutePages()
})