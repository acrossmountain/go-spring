/*
 * Copyright 2012-2019 the original author or authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package gs 实现了 go-spring 框架的骨架。
package gs

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime/debug"
	"sync"

	"github.com/go-spring/spring-core/arg"
	"github.com/go-spring/spring-core/bean"
	"github.com/go-spring/spring-core/conf"
	"github.com/go-spring/spring-core/log"
	"github.com/go-spring/spring-core/sort"
	"github.com/go-spring/spring-core/util"
)

type refreshState int

const (
	Unrefreshed = refreshState(0) // 未刷新
	Refreshing  = refreshState(1) // 正在刷新
	Refreshed   = refreshState(2) // 已刷新
)

// Container 实现了功能完善的 IoC 容器。
type Container struct {
	p *conf.Properties

	state refreshState

	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc

	beans       []*BeanDefinition
	beansById   map[string]*BeanDefinition
	beansByName map[string][]*BeanDefinition
	beansByType map[reflect.Type][]*BeanDefinition

	configers    []*Configer
	configerList *list.List

	destroyerList *list.List
}

type newArg struct {
	openPandora bool
}

type NewOption func(arg *newArg)

// OpenPandora 注册 Pandora 实例。
func OpenPandora() NewOption {
	return func(arg *newArg) {
		arg.openPandora = true
	}
}

// New 返回创建的 IoC 容器实例。
func New(opts ...NewOption) *Container {
	ctx, cancel := context.WithCancel(context.Background())

	a := newArg{}
	for _, opt := range opts {
		opt(&a)
	}

	c := &Container{
		p:             conf.New(),
		ctx:           ctx,
		cancel:        cancel,
		beansById:     make(map[string]*BeanDefinition),
		beansByName:   make(map[string][]*BeanDefinition),
		beansByType:   make(map[reflect.Type][]*BeanDefinition),
		configerList:  list.New(),
		destroyerList: list.New(),
	}

	if a.openPandora {
		c.Object(&pandora{c}).Export((*Pandora)(nil))
	}
	return c
}

// callAfterRefreshing 有些方法必须在 Refresh 开始后才能调用，比如 get、wire 等。
func (c *Container) callAfterRefreshing() {
	if c.state == Unrefreshed {
		panic(errors.New("should call after Refreshing"))
	}
}

// callBeforeRefreshing 有些方法在 Refresh 开始后不能再调用，比如 Object、Config 等。
func (c *Container) callBeforeRefreshing() {
	if c.state != Unrefreshed {
		panic(errors.New("should call before Refreshing"))
	}
}

// Load 从文件读取属性列表。
func (c *Container) Load(filename string) error {
	c.callBeforeRefreshing()
	return c.p.Load(filename)
}

// Property 设置 key 对应的属性值。
func (c *Container) Property(key string, value interface{}) {
	c.callBeforeRefreshing()
	c.p.Set(key, value)
}

// Object 注册对象形式的 bean 。
func (c *Container) Object(i interface{}) *BeanDefinition {
	c.callBeforeRefreshing()
	b := NewBean(reflect.ValueOf(i))
	return c.Register(b)
}

// Provide 注册构造函数形式的 bean 。
func (c *Container) Provide(ctor interface{}, args ...arg.Arg) *BeanDefinition {
	c.callBeforeRefreshing()
	b := NewBean(ctor, args...)
	return c.Register(b)
}

// Register 注册元数据形式的 bean 。
func (c *Container) Register(b *BeanDefinition) *BeanDefinition {
	c.callBeforeRefreshing()
	c.beans = append(c.beans, b)
	return b
}

// Config 注册一个配置函数
func (c *Container) Config(fn interface{}, args ...arg.Arg) *Configer {
	c.callBeforeRefreshing()

	t := reflect.TypeOf(fn)
	if !util.IsFuncType(t) {
		panic(errors.New("xxx"))
	}

	if !util.ReturnNothing(t) && !util.ReturnOnlyError(t) {
		panic(errors.New("fn should be func() or func()error"))
	}

	configer := &Configer{fn: arg.Bind(fn, args, arg.Skip(1))}
	c.configers = append(c.configers, configer)
	return configer
}

// find 返回符合条件的 bean 集合，不保证返回的 bean 已经完成注入和绑定过程。
func (c *Container) find(selector bean.Selector) ([]bean.Definition, error) {
	c.callAfterRefreshing()

	finder := func(fn func(*BeanDefinition) bool) (result []bean.Definition, err error) {
		for _, b := range c.beansById {
			if b.status != Resolving && fn(b) {
				// 避免 bean 未被解析
				if err = c.resolveBean(b); err != nil {
					return nil, err
				}
				if b.status != Deleted {
					result = append(result, b)
				}
			}
		}
		return
	}

	t := reflect.TypeOf(selector)

	if t.Kind() == reflect.String {
		tag := parseSingletonTag(selector.(string))
		return finder(func(b *BeanDefinition) bool {
			return b.Match(tag.typeName, tag.beanName)
		})
	}

	if t.Kind() == reflect.Ptr {
		if e := t.Elem(); e.Kind() == reflect.Interface {
			t = e // 接口类型去掉指针
		}
	}

	return finder(func(b *BeanDefinition) bool {
		if !b.Type().AssignableTo(t) {
			return false
		}
		if t.Kind() != reflect.Interface {
			return true
		}
		_, ok := b.exports[t]
		return ok
	})
}

// Refresh 对所有 bean 进行依赖注入和属性绑定
func (c *Container) Refresh() {

	if c.state != Unrefreshed {
		panic(errors.New("already refreshed"))
	}

	c.state = Refreshing

	c.registerBeans()
	c.resolveConfigers()

	err := c.resolveBeans()
	util.Panic(err).When(err != nil)

	assembly := toAssembly(c)

	defer func() {
		if len(assembly.stack) > 0 {
			log.Infof("wiring path %s", assembly.stack.path())
		}
	}()

	err = c.runConfigers(assembly)
	util.Panic(err).When(err != nil)

	err = c.wireBeans(assembly)
	util.Panic(err).When(err != nil)

	c.destroyerList = assembly.sortDestroyers()
	c.state = Refreshed

	log.Info("container refreshed successfully")
}

func (c *Container) registerBeans() {
	for _, b := range c.beans {
		if d, ok := c.beansById[b.ID()]; ok {
			panic(fmt.Errorf("found duplicate beans [%s] [%s]", b.Description(), d.Description()))
		}
		c.beansById[b.ID()] = b
	}
}

// resolveConfigers 对 Config 函数进行决议是否能够保留它
func (c *Container) resolveConfigers() {

	for _, g := range c.configers {
		if g.cond == nil || g.cond.Matches(&pandora{c}) {
			c.configerList.PushBack(g)
		}
	}

	c.configerList = sort.Triple(c.configerList, getBeforeConfigers)
}

// resolveBeans 对 bean 进行决议是否能够创建 bean 的实例。
func (c *Container) resolveBeans() error {
	for _, b := range c.beansById {
		if err := c.resolveBean(b); err != nil {
			return err
		}
	}
	return nil
}

// resolveBean 对 bean 进行决议是否能够创建 bean 的实例。
func (c *Container) resolveBean(b *BeanDefinition) error {

	// 正在进行或者已经完成决议过程
	if b.status >= Resolving {
		return nil
	}
	b.status = Resolving

	// 不满足判断条件的则标记为删除状态并删除其注册
	if b.cond != nil && !b.cond.Matches(&pandora{c}) {
		delete(c.beansById, b.ID())
		b.status = Deleted
		return nil
	}

	// 将符合注册条件的 bean 放入到缓存里面。
	log.Debugf("register %s name:%q type:%q %s", b.getClass(), b.Name(), b.Type().String(), b.FileLine())
	c.beansByType[b.Type()] = append(c.beansByType[b.Type()], b)

	// 自动导出接口，这种情况仅对于结构体才会有效
	if typ := util.Indirect(b.Type()); typ.Kind() == reflect.Struct {
		if err := c.autoExport(typ, b); err != nil {
			return err
		}
	}

	// 按照导出类型放入缓存
	for t := range b.exports {
		if b.Type().Implements(t) {
			log.Debugf("register %s name:%q type:%q %s", b.getClass(), b.Name(), t.String(), b.FileLine())
			c.beansByType[t] = append(c.beansByType[t], b)
		} else {
			return fmt.Errorf("%s not implement %s interface", b.Description(), t)
		}
	}

	// 按照 bean 的名字进行缓存。
	c.beansByName[b.name] = append(c.beansByName[b.name], b)

	b.status = Resolved
	return nil
}

// autoExport 自动导出 bean 实现的接口。
func (c *Container) autoExport(t reflect.Type, b *BeanDefinition) error {

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)

		_, export := f.Tag.Lookup("export")
		_, inject := f.Tag.Lookup("inject")
		_, autowire := f.Tag.Lookup("autowire")

		if !export { // 没有 export 标签

			// 嵌套才能递归；有注入标记的也不进行递归
			if !f.Anonymous || inject || autowire {
				continue
			}

			// 只处理结构体情况的递归，暂时不考虑接口的情况
			typ := util.Indirect(f.Type)
			if typ.Kind() == reflect.Struct {
				if err := c.autoExport(typ, b); err != nil {
					return err
				}
			}

			continue
		}

		// 有 export 标签的必须是接口类型
		if f.Type.Kind() != reflect.Interface {
			return errors.New("export can only use on interface")
		}

		// 不能导出需要注入的接口，因为会重复注册
		if export && (inject || autowire) {
			return errors.New("inject or autowire can't use with export")
		}

		// 不限定导出接口字段必须是空白标识符，但建议使用空白标识符
		b.Export(f.Type)
	}
	return nil
}

func (c *Container) runConfigers(assembly *beanAssembly) error {
	for e := c.configerList.Front(); e != nil; e = e.Next() {
		g := e.Value.(*Configer)
		if _, err := g.fn.Call(assembly); err != nil {
			return err
		}
	}
	return nil
}

func (c *Container) wireBeans(assembly *beanAssembly) error {
	for _, b := range c.beansById {
		if err := assembly.wireBean(b); err != nil {
			return err
		}
	}
	return nil
}

// Close 关闭容器，此方法必须在 Refresh 之后调用。该方法会触发 ctx 的 Done 信
// 号，然后等待所有 goroutine 结束，最后按照被依赖先销毁的原则执行所有的销毁函数。
func (c *Container) Close() {
	c.callAfterRefreshing()

	c.cancel()
	c.wg.Wait()

	log.Info("goroutines exited")

	assembly := toAssembly(c)
	c.runDestroyers(assembly)

	log.Info("container closed")
}

func (c *Container) runDestroyers(assembly *beanAssembly) {
	for e := c.destroyerList.Front(); e != nil; e = e.Next() {
		d := e.Value.(*destroyer)
		if err := d.run(assembly); err != nil {
			log.Error(err)
		}
	}
}

// Go 创建安全可等待的 goroutine，fn 要求的 ctx 对象由 Container 提供，当
// Container 关闭时 ctx 发出 Done 信号， fn 在接收到此信号后应当立即退出。
func (c *Container) Go(fn func(ctx context.Context)) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()

		defer func() {
			if r := recover(); r != nil {
				log.Errorf("%v, %s", r, debug.Stack())
			}
		}()

		fn(c.ctx)
	}()
}
